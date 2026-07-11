package controller

import (
	"strings"

	"github.com/elesos/tokenHub/common"
	"github.com/elesos/tokenHub/setting/system_setting"
)

func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}
