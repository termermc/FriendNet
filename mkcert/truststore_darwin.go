// Copyright 2018 The MkCert Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mkcert

import (
	"os"
	"path/filepath"
)

var (
	FirefoxProfiles     = []string{os.Getenv("HOME") + "/Library/Application Support/Firefox/Profiles/*"}
	CertutilInstallHelp = "brew Install nss"
	NSSBrowsers         = "Firefox"
)

func (m *MkCert) uninstallPlatform() bool {
	cmd := commandWithSudo("security", "remove-trusted-cert", "-d", filepath.Join(m.CAROOT, rootName))
	out, err := cmd.CombinedOutput()
	fatalIfCmdErr(err, "security remove-trusted-cert", out)

	return true
}
