package pairing

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
)

// pem is a namespace alias to avoid conflict with the standard "encoding/pem"
// import. We use a local package-level helper instead.

// encodeCSR PEM-encodes a DER certificate signing request.
func encodeCSRPEM(csr *x509.CertificateRequest) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csr.Raw,
	}))
}

// bigOne returns big.NewInt(1) for use as a cert serial number in stubs.
func bigOne() *big.Int { return big.NewInt(1) }

// parseCertChain converts a step-ca sign response into a *tls.Certificate.
// step-ca returns serverPEM (the leaf) and certChain (intermediate + root).
func parseCertChain(resp stepCASignResponse) (*tls.Certificate, error) {
	leafDER, err := decodePEMBlock(resp.ServerPEM.Raw, "CERTIFICATE")
	if err != nil {
		return nil, fmt.Errorf("parse leaf cert: %w", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, fmt.Errorf("parse leaf x509: %w", err)
	}

	chain := [][]byte{leafDER}
	for i, entry := range resp.CertChainPEM {
		der, err := decodePEMBlock(entry.Raw, "CERTIFICATE")
		if err != nil {
			return nil, fmt.Errorf("parse cert chain[%d]: %w", i, err)
		}
		chain = append(chain, der)
	}

	return &tls.Certificate{
		Certificate: chain,
		Leaf:        leaf,
	}, nil
}

func decodePEMBlock(pemStr, expectedType string) ([]byte, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found (expected %s)", expectedType)
	}
	if block.Type != expectedType {
		return nil, fmt.Errorf("unexpected PEM type %q (expected %q)", block.Type, expectedType)
	}
	return block.Bytes, nil
}
