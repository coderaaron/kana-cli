package setup

import (
	"os"

	"github.com/ChrisWiegman/kana/internal/config"
	"github.com/ChrisWiegman/kana/pkg/minica"
)

func EnsureCerts(kanaConfig config.KanaConfig) {

	if err := os.MkdirAll(kanaConfig.SSLCerts.CertDirectory, 0750); err != nil {
		panic(err)
	}

	minica.GenCerts(kanaConfig)
}
