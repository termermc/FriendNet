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
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sync"
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
	if runtime.GOOS == "darwin" {
		// Non-interactive sudo is impossible in MacOS.
		// Prompt the user to run the process as root.

		script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button 1`, "Please run the uninstall command as root", "Requires Root Permission")
		cmd := exec.Command("osascript", "-e", script)
		_ = cmd.Run()
		os.Exit(1)
		return nil
	}

	if u, err := user.Current(); err == nil && u.Uid == "0" {
		return exec.Command(cmd[0], cmd[1:]...)
	}

	if binaryExists("pkexec") {
		return exec.Command("pkexec", cmd...)
	}

	if binaryExists("sudo") {
		return exec.Command("sudo", append([]string{"--prompt=Sudo password:", "--"}, cmd...)...)
	}

	sudoWarningOnce.Do(func() {
		log.Println(`Warning: "sudo" is not available, and the program is not running as root. The (un)Install operation might fail.️`)
	})
	return exec.Command(cmd[0], cmd[1:]...)
}
