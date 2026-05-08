//go:build !windows && !linux && !darwin

// Copyright 2018 The MkCert Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mkcert

import (
	"fmt"
	"os"
	"runtime"
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
	CertutilInstallHelp string
)

func (m *MkCert) systemTrustFilename() string {
	return fmt.Sprintf(SystemTrustFilename, strings.Replace(m.caUniqueName(), " ", "_", -1))
}

func (m *MkCert) installPlatform() bool {
	println("Truststore not implemented for " + runtime.GOOS)

	return true
}

func (m *MkCert) uninstallPlatform() bool {
	println("Truststore not implemented for " + runtime.GOOS)

	return true
}
