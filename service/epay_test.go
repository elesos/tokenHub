package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/elesos/tokenHub/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestResolveEpayEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		endpoint string
		mode     epayMode
		wantErr  bool
	}{
		{
			name:     "base url",
			input:    "https://api.payqixiang.cn",
			endpoint: "https://api.payqixiang.cn/submit.php",
			mode:     epayModeSubmit,
		},
		{
			name:     "base url with trailing slash",
			input:    "https://api.payqixiang.cn/",
			endpoint: "https://api.payqixiang.cn/submit.php",
			mode:     epayModeSubmit,
		},
		{
			name:     "submit.php",
			input:    "https://api.payqixiang.cn/submit.php",
			endpoint: "https://api.payqixiang.cn/submit.php",
			mode:     epayModeSubmit,
		},
		{
			name:     "mapi.php",
			input:    "https://api.payqixiang.cn/mapi.php",
			endpoint: "https://api.payqixiang.cn/mapi.php",
			mode:     epayModeMapi,
		},
		{
			name:     "mapi.php with trailing slash",
			input:    "https://api.payqixiang.cn/mapi.php/",
			endpoint: "https://api.payqixiang.cn/mapi.php",
			mode:     epayModeMapi,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			input:   "api.payqixiang.cn",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			endpoint, mode, err := resolveEpayEndpoint(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.endpoint, endpoint)
			require.Equal(t, tt.mode, mode)
		})
	}
}

func TestBuildEpayPurchaseParamsIncludesClientIP(t *testing.T) {
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	params := buildEpayPurchaseParams(&EpayPurchaseArgs{
		Type:           "alipay",
		ServiceTradeNo: "USR1NOabc",
		Name:           "TUC10",
		Money:          "10.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "203.0.113.10",
	})

	require.Equal(t, "203.0.113.10", params["clientip"])
	require.Equal(t, "1001", params["pid"])
	require.Equal(t, "MD5", params["sign_type"])
	require.NotEmpty(t, params["sign"])

	// Re-sign with same key must match — proves clientip participates in signature.
	resign := epay.GenerateParams(map[string]string{
		"pid":          params["pid"],
		"type":         params["type"],
		"out_trade_no": params["out_trade_no"],
		"notify_url":   params["notify_url"],
		"name":         params["name"],
		"money":        params["money"],
		"device":       params["device"],
		"sign_type":    "MD5",
		"return_url":   params["return_url"],
		"clientip":     params["clientip"],
	}, "testkey")
	require.Equal(t, resign["sign"], params["sign"])
}

func TestEpayPurchaseMapiReturnsPayURL(t *testing.T) {
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	var gotClientIP string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		values, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		gotClientIP = values.Get("clientip")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":   1,
			"msg":    "success",
			"payurl": "https://pay.example.com/cashier/abc",
		})
	}))
	t.Cleanup(server.Close)

	operation_setting.PayAddress = server.URL + "/mapi.php"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	result, err := EpayPurchase(&EpayPurchaseArgs{
		Type:           "alipay",
		ServiceTradeNo: "USR1NOabc",
		Name:           "TUC10",
		Money:          "10.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "198.51.100.7",
	})
	require.NoError(t, err)
	require.Equal(t, "198.51.100.7", gotClientIP)
	require.Equal(t, "https://pay.example.com/cashier/abc", result.URL)
	require.Equal(t, EpayLaunchURL, result.Type)
	require.Empty(t, result.Params)
}

func TestEpayPurchaseMapiReturnsQRCodeType(t *testing.T) {
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":   1,
			"msg":    "success",
			"qrcode": "https://qr.alipay.com/bax03070g7gteuwpysvd008c",
		})
	}))
	t.Cleanup(server.Close)

	operation_setting.PayAddress = server.URL + "/mapi.php"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	result, err := EpayPurchase(&EpayPurchaseArgs{
		Type:           "alipay",
		ServiceTradeNo: "USR1NOqr",
		Name:           "TUC10",
		Money:          "10.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "198.51.100.7",
	})
	require.NoError(t, err)
	require.Equal(t, "https://qr.alipay.com/bax03070g7gteuwpysvd008c", result.URL)
	require.Equal(t, EpayLaunchQRCode, result.Type)
	require.Empty(t, result.Params)
}

func TestEpayPurchaseMapiReclassifiesAlipayPayURLAsQRCode(t *testing.T) {
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":   1,
			"msg":    "success",
			"payurl": "https://qr.alipay.com/bax03070g7gteuwpysvd008c",
		})
	}))
	t.Cleanup(server.Close)

	operation_setting.PayAddress = server.URL + "/mapi.php"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	result, err := EpayPurchase(&EpayPurchaseArgs{
		Type:           "alipay",
		ServiceTradeNo: "USR1NOpayurlqr",
		Name:           "TUC10",
		Money:          "10.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "198.51.100.7",
	})
	require.NoError(t, err)
	require.Equal(t, EpayLaunchQRCode, result.Type)
}

func TestLooksLikePaymentQRContent(t *testing.T) {
	t.Parallel()
	require.True(t, looksLikePaymentQRContent("https://qr.alipay.com/bax123"))
	require.True(t, looksLikePaymentQRContent("https://render.alipay.com/p/s/i?scheme=alipays%3A%2F%2F"))
	require.True(t, looksLikePaymentQRContent("weixin://wxpay/bizpayurl?pr=xxx"))
	require.True(t, looksLikePaymentQRContent("alipays://platformapi/startapp?saId=10000007"))
	require.False(t, looksLikePaymentQRContent("https://pay.example.com/cashier/abc"))
	require.False(t, looksLikePaymentQRContent(""))
}

func TestEpayPurchaseSubmitIncludesClientIP(t *testing.T) {
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	operation_setting.PayAddress = "https://api.payqixiang.cn"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	result, err := EpayPurchase(&EpayPurchaseArgs{
		Type:           "wxpay",
		ServiceTradeNo: "USR2NOdef",
		Name:           "TUC5",
		Money:          "5.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "203.0.113.55",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.payqixiang.cn/submit.php", result.URL)
	require.Equal(t, EpayLaunchForm, result.Type)
	require.Equal(t, "203.0.113.55", result.Params["clientip"])
	require.NotEmpty(t, result.Params["sign"])
}

func TestEpayPurchaseMapiRequiresClientIP(t *testing.T) {
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})

	operation_setting.PayAddress = "https://api.payqixiang.cn/mapi.php"
	operation_setting.EpayId = "1001"
	operation_setting.EpayKey = "testkey"

	notify, err := url.Parse("https://example.com/notify")
	require.NoError(t, err)
	ret, err := url.Parse("https://example.com/return")
	require.NoError(t, err)

	_, err = EpayPurchase(&EpayPurchaseArgs{
		Type:           "alipay",
		ServiceTradeNo: "USR3NOghi",
		Name:           "TUC1",
		Money:          "1.00",
		Device:         epay.PC,
		NotifyUrl:      notify,
		ReturnUrl:      ret,
		ClientIP:       "",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "clientip")
}

func TestEpayMapiCodeOK(t *testing.T) {
	t.Parallel()
	require.True(t, epayMapiCodeOK(float64(1)))
	require.True(t, epayMapiCodeOK(1))
	require.True(t, epayMapiCodeOK("1"))
	require.False(t, epayMapiCodeOK(float64(-1)))
	require.False(t, epayMapiCodeOK(nil))
}
