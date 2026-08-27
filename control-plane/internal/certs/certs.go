// Package certs issues a certificate for a node without ever seeing its key.
//
// The node makes its own key and sends only a signing request. This service
// proves we own the domain, asks the authority to sign, and hands back the
// certificate. Nothing that could impersonate the node travels in either
// direction, and nothing that could edit our DNS ever reaches a node.
//
// That asymmetry is the point of the whole arrangement. Taking a node means
// taking one certificate, for one machine, replaced by destroying that machine.
// It does not mean the certificates of the other nodes, and it does not mean
// the domains.
package certs

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"download.simplevpn/control-plane/internal/dnsedit"
)

// Issuer turns a signing request into a certificate.
type Issuer struct {
	account *account
	dns     *dnsedit.Editor
	log     *slog.Logger

	// directory is the authority to ask. Configurable so that a rehearsal can
	// use the staging one, whose certificates nobody trusts and whose limits
	// nobody counts - which is where a change to this code belongs first.
	directory string
}

// account is what the authority knows us as.
//
// Held rather than created per request: an authority that saw a new account
// for every certificate would be watching somebody behave like an attack.
type account struct {
	key          crypto.PrivateKey
	registration *registration.Resource
}

func (a *account) GetEmail() string { return "" }

func (a *account) GetRegistration() *registration.Resource { return a.registration }

func (a *account) GetPrivateKey() crypto.PrivateKey { return a.key }

// New prepares an issuer from a stored account key.
//
// The key is configuration rather than something generated here, because an
// account that changed on every restart would leave a trail of abandoned
// registrations and lose the standing that rate limits are counted against.
func New(accountKeyPEM, directory string, dns *dnsedit.Editor, log *slog.Logger) (*Issuer, error) {
	if strings.TrimSpace(accountKeyPEM) == "" {
		return nil, errors.New("no ACME account key")
	}

	// Accepted either as PEM or as base64 of it. A private key is several
	// lines and the environment file that carries it is one line per value, so
	// the encoded form is the one that survives the journey.
	material := strings.TrimSpace(accountKeyPEM)
	if !strings.HasPrefix(material, "-----") {
		decoded, err := base64.StdEncoding.DecodeString(material)
		if err != nil {
			return nil, fmt.Errorf("the ACME account key is neither PEM nor base64: %w", err)
		}
		material = string(decoded)
	}

	block, _ := pem.Decode([]byte(material))
	if block == nil {
		return nil, errors.New("the ACME account key is not PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("the ACME account key is unreadable: %w", err)
	}

	if directory == "" {
		directory = lego.LEDirectoryProduction
	}

	return &Issuer{
		account:   &account{key: key},
		dns:       dns,
		log:       log,
		directory: directory,
	}, nil
}

// Issue proves the domain and returns the signed certificate chain.
//
// The request carries the node's public key and the name it wants; the private
// half stays where it was made. What comes back is public by definition - a
// certificate is meant to be shown to everybody - so the answer needs no
// protection beyond being the right one.
func (i *Issuer) Issue(ctx context.Context, csrPEM string) (string, error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return "", errors.New("the request is not PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("the request is unreadable: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		// A request the sender cannot prove it made is a request to sign
		// somebody else's key. Refused before anything is published.
		return "", fmt.Errorf("the request is not signed by its own key: %w", err)
	}

	config := lego.NewConfig(i.account)
	config.CADirURL = i.directory
	config.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(config)
	if err != nil {
		return "", fmt.Errorf("cannot reach the authority: %w", err)
	}

	if err := client.Challenge.SetDNS01Provider(
		&solver{dns: i.dns, log: i.log},
		// Waits for the record to be visible from the authoritative servers
		// rather than trusting the moment the API accepted it. A record
		// accepted and not yet served is the ordinary way this fails.
		dns01.AddDNSTimeout(3*time.Minute),
	); err != nil {
		return "", fmt.Errorf("cannot set up the proof: %w", err)
	}

	if i.account.registration == nil {
		reg, err := client.Registration.Register(
			registration.RegisterOptions{TermsOfServiceAgreed: true},
		)
		if err != nil {
			// An account that already exists is not an error: it is the
			// ordinary case after the first certificate.
			reg, err = client.Registration.ResolveAccountByKey()
			if err != nil {
				return "", fmt.Errorf("cannot register with the authority: %w", err)
			}
		}
		i.account.registration = reg
	}

	resource, err := client.Certificate.ObtainForCSR(certificate.ObtainForCSRRequest{
		CSR: csr,
		// The chain, so that a node serves what a browser needs without having
		// to fetch anything itself.
		Bundle: true,
	})
	if err != nil {
		return "", fmt.Errorf("the authority did not issue: %w", err)
	}

	return string(resource.Certificate), nil
}

// NamesIn reports what a request asks to be certified.
//
// Read before anything is published, because publishing a proof for a name we
// were not asked about would be answering a question nobody posed.
func NamesIn(csrPEM string) ([]string, error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return nil, errors.New("the request is not PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("the request is unreadable: %w", err)
	}

	names := map[string]bool{}
	if csr.Subject.CommonName != "" {
		names[strings.ToLower(csr.Subject.CommonName)] = true
	}
	for _, name := range csr.DNSNames {
		names[strings.ToLower(name)] = true
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	return out, nil
}

// solver publishes the proof through whichever registrar holds the domain.
type solver struct {
	dns *dnsedit.Editor
	log *slog.Logger
}

func (s *solver) Present(domain, _, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	s.log.Info("publishing the proof", "domain", domain)
	return s.dns.Present(context.Background(), domain, info.Value)
}

func (s *solver) CleanUp(domain, _, _ string) error {
	// Failure here does not withdraw a certificate that has been granted. The
	// record proves nothing to anybody who does not already control the
	// domain; it is removed so that a name littered with old proofs does not
	// tell a reader how often we issue.
	if err := s.dns.CleanUp(context.Background(), domain); err != nil {
		s.log.Warn("could not remove the proof", "domain", domain, "error", err)
	}
	return nil
}
