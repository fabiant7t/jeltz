package ca

// PKCS#12 encoding (RFC 7292) using only Go standard library.
//
// Format: PFX v3 with:
//   - unencrypted CertBag (cert safe bag)
//   - PBE-SHA1-3DES ShroudedKeyBag (key safe bag, RFC 7292 Appendix B KDF)
//   - HMAC-SHA1 MAC (RFC 7292 Appendix B KDF, ID=3)
//   - password "jeltz" encoded as BMPString (UTF-16BE + null terminator)

import (
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"os"
)

// OIDs used in PKCS#12 encoding.
var (
	oidP12Data     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidP12CertBag  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidP12KeyBag   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidP12X509Cert = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}
	oidP12KeyID    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 21}
	oidPBE3DES     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	oidSHA1        = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
)

// ---- ASN.1 structures ---------------------------------------------------

type p12PFX struct {
	Version  int
	AuthSafe p12ContentInfo
	MacData  p12MacData
}

// p12ContentInfo represents a PKCS#7 ContentInfo.
// Content is encoded as [0] EXPLICIT manually (see explicit0) because
// encoding/asn1 ignores struct tags when RawValue.FullBytes is set.
type p12ContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"optional"`
}

// p12SafeBag represents a PKCS#12 SafeBag.
// Value is encoded as [0] EXPLICIT manually (see explicit0).
type p12SafeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue  `asn1:"optional"`
	Attributes []p12Attribute `asn1:"set,optional"`
}

type p12Attribute struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue
}

// p12CertBag represents a PKCS#12 CertBag.
// Value is encoded as [0] EXPLICIT manually (see explicit0).
type p12CertBag struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"optional"`
}

type p12EncryptedPKI struct {
	Algorithm pkix.AlgorithmIdentifier
	Data      []byte
}

type p12PBEParams struct {
	Salt       []byte
	Iterations int
}

type p12MacData struct {
	Mac        p12DigestInfo
	Salt       []byte
	Iterations int `asn1:"optional,default:1"`
}

type p12DigestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

// P12Password is the fixed password used for all jeltz PKCS#12 bundles.
const P12Password = "jeltz"

// p12PasswordBytes converts P12Password to the BMPString (UTF-16BE + null
// terminator) representation required by the PKCS#12 KDF (RFC 7292).
func p12PasswordBytes() []byte {
	s := P12Password
	b := make([]byte, len(s)*2+2)
	for i, c := range s {
		b[i*2] = byte(c >> 8)
		b[i*2+1] = byte(c)
	}
	// null terminator
	b[len(s)*2] = 0
	b[len(s)*2+1] = 0
	return b
}

// explicit0 wraps inner DER bytes in a [0] EXPLICIT context tag.
// Go's encoding/asn1 ignores struct tags when RawValue.FullBytes is set,
// so all [0] EXPLICIT wrappers must be constructed manually this way.
func explicit0(inner []byte) asn1.RawValue {
	return asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: inner}
}

// ---- Entry point --------------------------------------------------------

// writeP12 encodes key and cert as a PKCS#12 PFX bundle (password: P12Password)
// and writes it to path with mode 0600. Only stdlib crypto is used.
func writeP12(path string, key *rsa.PrivateKey, cert *x509.Certificate) error {
	data, err := encodePKCS12(key, cert)
	if err != nil {
		return fmt.Errorf("ca: encode p12: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ---- Encoding -----------------------------------------------------------

// encodePKCS12 produces a DER-encoded PKCS#12 PFX bundle.
func encodePKCS12(key *rsa.PrivateKey, cert *x509.Certificate) ([]byte, error) {
	password := p12PasswordBytes()

	// LocalKeyID = SHA-1 of the certificate DER, used to link cert and key.
	keyID := sha1.Sum(cert.Raw)

	// 1. Cert SafeBag (unencrypted).
	certBagDER, err := marshalCertBag(cert, keyID[:])
	if err != nil {
		return nil, fmt.Errorf("cert bag: %w", err)
	}
	certSafeContents, err := wrapInSequence(certBagDER)
	if err != nil {
		return nil, err
	}
	certCI, err := makeDataCI(certSafeContents)
	if err != nil {
		return nil, err
	}

	// 2. Key SafeBag (3DES encrypted).
	keyBagDER, err := marshalKeyBag(key, password, keyID[:])
	if err != nil {
		return nil, fmt.Errorf("key bag: %w", err)
	}
	keySafeContents, err := wrapInSequence(keyBagDER)
	if err != nil {
		return nil, err
	}
	keyCI, err := makeDataCI(keySafeContents)
	if err != nil {
		return nil, err
	}

	// 3. AuthenticatedSafe = SEQUENCE OF ContentInfo.
	authSafeDER, err := asn1.Marshal([]p12ContentInfo{certCI, keyCI})
	if err != nil {
		return nil, fmt.Errorf("authSafe: %w", err)
	}

	// 4. MAC over the DER encoding of AuthenticatedSafe.
	mac, err := computeP12MAC(authSafeDER, password)
	if err != nil {
		return nil, err
	}

	// 5. Outer ContentInfo wrapping AuthenticatedSafe in an OCTET STRING.
	outerCI, err := makeDataCI(authSafeDER)
	if err != nil {
		return nil, err
	}

	return asn1.Marshal(p12PFX{
		Version:  3,
		AuthSafe: outerCI,
		MacData:  mac,
	})
}

// marshalCertBag encodes cert as a PKCS#12 CertBag SafeBag.
func marshalCertBag(cert *x509.Certificate, keyID []byte) ([]byte, error) {
	// Inner OCTET STRING contains the raw DER certificate.
	certOctetDER, err := asn1.Marshal(cert.Raw)
	if err != nil {
		return nil, err
	}
	// CertBag ::= SEQUENCE { certId OID, certValue [0] EXPLICIT OCTET STRING }
	certBagDER, err := asn1.Marshal(p12CertBag{
		ID:    oidP12X509Cert,
		Value: explicit0(certOctetDER), // [0] EXPLICIT { OCTET STRING }
	})
	if err != nil {
		return nil, err
	}
	attr, err := localKeyIDAttr(keyID)
	if err != nil {
		return nil, err
	}
	// SafeBag ::= SEQUENCE { bagId OID, bagValue [0] EXPLICIT CertBag, bagAttributes SET }
	return asn1.Marshal(p12SafeBag{
		ID:         oidP12CertBag,
		Value:      explicit0(certBagDER), // [0] EXPLICIT { CertBag }
		Attributes: []p12Attribute{attr},
	})
}

// marshalKeyBag encrypts key with PBE-SHA1-3DES and encodes it as a
// PKCS#12 ShroudedKeyBag SafeBag.
func marshalKeyBag(key *rsa.PrivateKey, password, keyID []byte) ([]byte, error) {
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}

	const iters = 2048
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	// Derive key and IV via PKCS#12 KDF (RFC 7292 Appendix B).
	encKey := p12KDF(1, password, salt, iters, 24) // 24-byte 3DES key
	encIV := p12KDF(2, password, salt, iters, 8)   // 8-byte IV

	block, err := des.NewTripleDESCipher(encKey)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(pkcs8DER, block.BlockSize())
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, encIV).CryptBlocks(ct, padded)

	paramsDER, err := asn1.Marshal(p12PBEParams{Salt: salt, Iterations: iters})
	if err != nil {
		return nil, err
	}
	alg := pkix.AlgorithmIdentifier{
		Algorithm:  oidPBE3DES,
		Parameters: asn1.RawValue{FullBytes: paramsDER},
	}
	epkiDER, err := asn1.Marshal(p12EncryptedPKI{Algorithm: alg, Data: ct})
	if err != nil {
		return nil, err
	}

	attr, err := localKeyIDAttr(keyID)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(p12SafeBag{
		ID:         oidP12KeyBag,
		Value:      explicit0(epkiDER), // [0] EXPLICIT { EncryptedPrivateKeyInfo }
		Attributes: []p12Attribute{attr},
	})
}

// makeDataCI creates a pkcs-7-data ContentInfo wrapping data in an OCTET STRING.
func makeDataCI(data []byte) (p12ContentInfo, error) {
	// Content ::= [0] EXPLICIT OCTET STRING { data }.
	// The [0] EXPLICIT wrapper is built manually via explicit0 because
	// encoding/asn1 ignores struct tags when RawValue.FullBytes is set.
	octetDER, err := asn1.Marshal(data)
	if err != nil {
		return p12ContentInfo{}, err
	}
	return p12ContentInfo{
		ContentType: oidP12Data,
		Content:     explicit0(octetDER), // [0] EXPLICIT { OCTET STRING }
	}, nil
}

// localKeyIDAttr builds a PKCS#12 localKeyId attribute (SET { OCTET STRING }).
func localKeyIDAttr(keyID []byte) (p12Attribute, error) {
	keyIDDER, err := asn1.Marshal(keyID)
	if err != nil {
		return p12Attribute{}, err
	}
	return p12Attribute{
		ID: oidP12KeyID,
		// Value is SET { OCTET STRING(keyID) }.
		Value: asn1.RawValue{
			Tag:        17,       // SET
			Class:      0,        // UNIVERSAL
			IsCompound: true,
			Bytes:      keyIDDER, // SET content: the OCTET STRING
		},
	}, nil
}

// wrapInSequence wraps the DER bytes of a single element into a SEQUENCE.
func wrapInSequence(elemDER []byte) ([]byte, error) {
	return asn1.Marshal(asn1.RawValue{
		Tag:        16, // SEQUENCE
		IsCompound: true,
		Bytes:      elemDER,
	})
}

// computeP12MAC computes HMAC-SHA1 over data using the PKCS#12 KDF (ID=3).
func computeP12MAC(data, password []byte) (p12MacData, error) {
	const iters = 2048
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return p12MacData{}, err
	}
	macKey := p12KDF(3, password, salt, iters, 20) // 20-byte HMAC-SHA1 key
	mac := hmac.New(sha1.New, macKey)
	mac.Write(data)
	return p12MacData{
		Mac: p12DigestInfo{
			Algorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidSHA1,
				Parameters: asn1.RawValue{Tag: 5}, // NULL
			},
			Digest: mac.Sum(nil),
		},
		Salt:       salt,
		Iterations: iters,
	}, nil
}

// ---- PKCS#12 Key Derivation Function (RFC 7292 Appendix B, SHA-1) -------

// p12KDF derives keyLen bytes using the PKCS#12 KDF with SHA-1.
//
//	id=1: encryption key
//	id=2: IV
//	id=3: MAC key
func p12KDF(id byte, password, salt []byte, iterations, keyLen int) []byte {
	const v = 64 // SHA-1 block size in bytes
	const u = 20 // SHA-1 output size in bytes

	D := make([]byte, v)
	for i := range D {
		D[i] = id
	}

	S := p12Expand(salt, v)
	P := p12Expand(password, v)
	I := append(S, P...)

	B := make([]byte, v)
	result := make([]byte, 0, keyLen)

	for len(result) < keyLen {
		// A = H^iterations(D || I)
		h := sha1.New()
		h.Write(D)
		h.Write(I)
		A := h.Sum(nil)
		for i := 1; i < iterations; i++ {
			h.Reset()
			h.Write(A)
			A = h.Sum(nil)
		}
		result = append(result, A...)

		// B = A repeated to fill v bytes.
		for j := range B {
			B[j] = A[j%u]
		}
		// I_j = (I_j + B + 1) mod 2^(v*8) for each v-byte chunk of I.
		for j := 0; j < len(I); j += v {
			p12AddB(I[j:j+v], B)
		}
	}
	return result[:keyLen]
}

// p12Expand repeats data until the output length is the smallest multiple of v
// that is >= len(data). Returns nil if data is empty.
func p12Expand(data []byte, v int) []byte {
	if len(data) == 0 {
		return nil
	}
	n := v * ((len(data) + v - 1) / v)
	out := make([]byte, n)
	for i := range out {
		out[i] = data[i%len(data)]
	}
	return out
}

// p12AddB computes a = (a + B + 1) mod 2^(len(a)*8) in-place (big-endian).
func p12AddB(a, B []byte) {
	carry := 1
	for i := len(a) - 1; i >= 0; i-- {
		sum := int(a[i]) + int(B[i]) + carry
		a[i] = byte(sum)
		carry = sum >> 8
	}
}

// ---- PKCS#7 padding -----------------------------------------------------

// pkcs7Pad pads data to a multiple of blockSize bytes using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}
