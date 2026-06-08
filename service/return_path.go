package service

import (
	"strings"

	"github.com/QuantumNous/tokenHub/common"
	"github.com/QuantumNous/tokenHub/setting/system_setting"
)

func PaymentReturnURL(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}
