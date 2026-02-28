// Copyright 2018 The MkCert Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command MkCert is a simple zero-config tool to make development certificates.
package mkcert

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"sync"

	"golang.org/x/net/idna"
)

const rootName = "rootCA.pem"
const rootKeyName = "rootCA-key.pem"

// MkCert is the mkcert CLI.
//
// Set InstallMode to true to Install the global cert.
// Set UninstallMode to true to Uninstall the global cert.
type MkCert struct {
	InstallMode, UninstallMode bool

	CAROOT string
	caCert *x509.Certificate
	caKey  crypto.PrivateKey

	// The system cert pool is only loaded once. After installing the root, checks
	// will keep failing until the next execution. TODO: maybe execve?
	// https://github.com/golang/go/issues/24540 (thanks, myself)
	ignoreCheckFailure bool
}

// Wrapper around the mkcert init functions for each file.
func initGlobalState() {
	initCertGo()
	initTruststoreLinuxGo()
	initTruststoreNssGo()
}

func NewMkCert(caRootDir string) (*MkCert, error) {
	m := &MkCert{
		CAROOT: caRootDir,
	}

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				err, ok = r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}
			}
		}()

		initGlobalState()

		m.loadCA()
	}()

	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *MkCert) GenCert(hostnames []string) (certPem []byte, privKeyPem []byte, err error) {
	hostnameRegexp := regexp.MustCompile(`(?i)^(\*\.)?[0-9a-z_-]([0-9a-z._-]*[0-9a-z_-])?$`)
	for i, name := range hostnames {
		if ip := net.ParseIP(name); ip != nil {
			continue
		}
		if email, err := mail.ParseAddress(name); err == nil && email.Address == name {
			continue
		}
		if uriName, err := url.Parse(name); err == nil && uriName.Scheme != "" && uriName.Host != "" {
			continue
		}
		punycode, err := idna.ToASCII(name)
		if err != nil {
			log.Fatalf("ERROR: %q is not a valid hostname, IP, URL or email: %s", name, err)
		}
		hostnames[i] = punycode
		if !hostnameRegexp.MatchString(punycode) {
			log.Fatalf("ERROR: %q is not a valid hostname, IP, URL or email", name)
		}
	}

	return m.makeCert(hostnames)
}

// Install installs the root CA to the system's trust store.
// Should be run in a terminal on Linux and Darwin because it tries to use `sudo`.
func (m *MkCert) Install() error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				err, ok = r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}
			}
		}()

		if storeEnabled("system") {
			if m.CheckPlatform() {
				log.Print("The local CA is already installed in the system trust store!")
			} else {
				if m.installPlatform() {
					log.Print("The local CA is now installed in the system trust store!️")
				}
				m.ignoreCheckFailure = true // TODO: replace with a check for a successful Install
			}
		}
		if storeEnabled("nss") && hasNSS {
			if m.CheckNSS() {
				log.Printf("The local CA is already installed in the %s trust store!", NSSBrowsers)
			} else {
				if hasCertutil && m.installNSS() {
					log.Printf("The local CA is now installed in the %s trust store (requires browser restart)!", NSSBrowsers)
				} else if CertutilInstallHelp == "" {
					log.Printf(`Note: %s support is not available on your platform.️`, NSSBrowsers)
				} else if !hasCertutil {
					log.Printf(`Warning: "certutil" is not available, so the CA can't be automatically installed in %s! `, NSSBrowsers)
					log.Printf(`Install "certutil" with "%s" then re-run`, CertutilInstallHelp)
				}
			}
		}
	}()

	return err
}

// Uninstall uninstalls the root CA to the system's trust store.
// Should be run in a terminal on Linux and Darwin because it tries to use `sudo`.
func (m *MkCert) Uninstall() error {
	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				err, ok = r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}
			}
		}()

		if storeEnabled("nss") && hasNSS {
			if hasCertutil {
				m.uninstallNSS()
			} else if CertutilInstallHelp != "" {
				log.Print("")
				log.Printf(`Warning: "certutil" is not available, so the CA can't be automatically uninstalled from %s (if it was ever installed)!️`, NSSBrowsers)
				log.Printf(`You can Install "certutil" with "%s" and re-run "-uninstallca"`, CertutilInstallHelp)
				log.Print("")
			}
		}
		if storeEnabled("system") && m.uninstallPlatform() {
			log.Print("The local CA is now uninstalled from the system trust store(s)!")
			log.Print("")
		} else if storeEnabled("nss") && hasCertutil {
			log.Printf("The local CA is now uninstalled from the %s trust store(s)!", NSSBrowsers)
			log.Print("")
		}
	}()

	return err
}

// CheckPlatform returns whether the local CA is installed in the system trust store.
func (m *MkCert) CheckPlatform() bool {
	if m.ignoreCheckFailure {
		return true
	}

	_, err := m.caCert.Verify(x509.VerifyOptions{})
	return err == nil
}

func storeEnabled(name string) bool {
	return true
}

func fatalIfErr(err error, msg string) {
	if err != nil {
		log.Fatalf("ERROR: %s: %s", msg, err)
	}
}

func fatalIfCmdErr(err error, cmd string, out []byte) {
	if err != nil {
		log.Fatalf("ERROR: failed to execute \"%s\": %s\n\n%s\n", cmd, err, out)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func binaryExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

var sudoWarningOnce sync.Once

func commandWithSudo(cmd ...string) *exec.Cmd {
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		return exec.Command(cmd[0], cmd[1:]...)
	}
	if !binaryExists("sudo") {
		sudoWarningOnce.Do(func() {
			log.Println(`Warning: "sudo" is not available, and the program is not running as root. The (un)Install operation might fail.️`)
		})
		return exec.Command(cmd[0], cmd[1:]...)
	}
	return exec.Command("sudo", append([]string{"--prompt=Sudo password:", "--"}, cmd...)...)
}
