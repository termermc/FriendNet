// Copyright 2018 The MkCert Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mkcert

import (
	"fmt"
	"os"
	"strings"
)

var (
	FirefoxProfiles = []string{
		os.Getenv("HOME") + "/.mozilla/firefox/*",
		os.Getenv("HOME") + "/.config/mozilla/firefox/*",
		os.Getenv("HOME") + "/.zen/*",
		os.Getenv("HOME") + "/.librewolf/*",
		os.Getenv("HOME") + "/.config/librewolf/librewolf/*",
		os.Getenv("HOME") + "/snap/firefox/common/.mozilla/firefox/*",
		os.Getenv("HOME") + "/.var/app/org.mozilla.firefox/.mozilla/firefox/*",
		os.Getenv("HOME") + "/.var/app/org.mozilla.firefox/.zen/*",
		os.Getenv("HOME") + "/.var/app/io.gitlab.librewolf-community/.librewolf/*",
	}
	NSSBrowsers = "Firefox and/or Chrome/Chromium"

	SystemTrustFilename string
	SystemTrustCommand  []string
	CertutilInstallHelp string
)

func initTruststoreLinuxGo() {
	switch {
	case binaryExists("apt"):
		CertutilInstallHelp = "apt Install libnss3-tools"
	case binaryExists("yum"):
		CertutilInstallHelp = "yum Install nss-tools"
	case binaryExists("zypper"):
		CertutilInstallHelp = "zypper Install mozilla-nss-tools"
	}
	if pathExists("/etc/pki/ca-trust/source/anchors/") {
		SystemTrustFilename = "/etc/pki/ca-trust/source/anchors/%s.pem"
		SystemTrustCommand = []string{"update-ca-trust", "extract"}
	} else if pathExists("/usr/local/share/ca-certificates/") {
		SystemTrustFilename = "/usr/local/share/ca-certificates/%s.crt"
		SystemTrustCommand = []string{"update-ca-certificates"}
	} else if pathExists("/etc/ca-certificates/trust-source/anchors/") {
		SystemTrustFilename = "/etc/ca-certificates/trust-source/anchors/%s.crt"
		SystemTrustCommand = []string{"trust", "extract-compat"}
	} else if pathExists("/usr/share/pki/trust/anchors") {
		SystemTrustFilename = "/usr/share/pki/trust/anchors/%s.pem"
		SystemTrustCommand = []string{"update-ca-certificates"}
	}
}

func (m *MkCert) systemTrustFilename() string {
	return fmt.Sprintf(SystemTrustFilename, strings.Replace(m.caUniqueName(), " ", "_", -1))
}

func (m *MkCert) uninstallPlatform() bool {
	if SystemTrustCommand == nil {
		return false
	}

	cmd := commandWithSudo("rm", "-f", m.systemTrustFilename())
	out, err := cmd.CombinedOutput()
	fatalIfCmdErr(err, "rm", out)

	// We used to Install under non-unique filenames.
	legacyFilename := fmt.Sprintf(SystemTrustFilename, "FriendNet-rootCA")
	if pathExists(legacyFilename) {
		cmd := commandWithSudo("rm", "-f", legacyFilename)
		out, err := cmd.CombinedOutput()
		fatalIfCmdErr(err, "rm (legacy filename)", out)
	}

	cmd = commandWithSudo(SystemTrustCommand...)
	out, err = cmd.CombinedOutput()
	fatalIfCmdErr(err, strings.Join(SystemTrustCommand, " "), out)

	return true
}
