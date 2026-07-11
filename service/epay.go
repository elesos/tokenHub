package service

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/tokenHub/common"
	"github.com/QuantumNous/tokenHub/setting/operation_setting"
	"github.com/QuantumNous/tokenHub/setting/system_setting"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}

// EpayPurchaseArgs is the input for building an EasyPay (易支付) order.
type EpayPurchaseArgs struct {
	Type           string
	ServiceTradeNo string
	Name           string
	Money          string
	Device         epay.DeviceType
	NotifyUrl      *url.URL
	ReturnUrl      *url.URL
	// ClientIP is required by mapi.php (API mode) and recommended for submit.php.
	ClientIP string
}

// EpayLaunchType tells the frontend how to present the payment result.
//
//   - form: POST Params to URL (submit.php page-jump)
//   - url: open/redirect to a browser cashier URL
//   - qrcode: render URL content as a QR code for mobile-app scan (Alipay/WeChat)
//   - urlscheme: mobile deep-link; open on mobile, otherwise show as QR when possible
type EpayLaunchType string

const (
	EpayLaunchForm      EpayLaunchType = "form"
	EpayLaunchURL       EpayLaunchType = "url"
	EpayLaunchQRCode    EpayLaunchType = "qrcode"
	EpayLaunchURLScheme EpayLaunchType = "urlscheme"
)

// EpayPurchaseResult is returned to the controller for frontend payment launch.
//
// Page-jump mode (submit.php): URL is the gateway form endpoint and Params are
// signed form fields. Frontend should POST the form (Type=form).
//
// API mode (mapi.php): Params is empty; Type indicates how to use URL:
// payurl → open, qrcode → show QR, urlscheme → open on mobile / QR on desktop.
type EpayPurchaseResult struct {
	URL    string
	Params map[string]string
	Type   EpayLaunchType
}

type epayMode string

const (
	epayModeSubmit epayMode = "submit"
	epayModeMapi   epayMode = "mapi"
)

// GetEpayClient creates an EasyPay client from configured options.
func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	client, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return client
}

// resolveEpayEndpoint normalizes the configured PayAddress into a concrete
// endpoint URL and mode.
//
// Supported PayAddress forms:
//   - https://pay.example.com              -> https://pay.example.com/submit.php (page jump)
//   - https://pay.example.com/submit.php   -> page jump as-is
//   - https://pay.example.com/mapi.php     -> API mode as-is
func resolveEpayEndpoint(payAddress string) (endpoint string, mode epayMode, err error) {
	payAddress = strings.TrimSpace(payAddress)
	if payAddress == "" {
		return "", "", fmt.Errorf("pay address is empty")
	}
	u, err := url.Parse(payAddress)
	if err != nil {
		return "", "", fmt.Errorf("invalid pay address: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid pay address: missing scheme or host")
	}

	// Normalize path: drop trailing slash for suffix checks.
	pathLower := strings.ToLower(strings.TrimRight(u.Path, "/"))
	switch {
	case strings.HasSuffix(pathLower, "mapi.php"):
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawQuery = ""
		u.Fragment = ""
		return u.String(), epayModeMapi, nil
	case strings.HasSuffix(pathLower, "submit.php"):
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawQuery = ""
		u.Fragment = ""
		return u.String(), epayModeSubmit, nil
	default:
		// Base gateway URL — page-jump endpoint.
		base := strings.TrimRight(payAddress, "/")
		return base + "/submit.php", epayModeSubmit, nil
	}
}

func buildEpayPurchaseParams(args *EpayPurchaseArgs) map[string]string {
	device := string(args.Device)
	if device == "" {
		device = string(epay.PC)
	}
	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"type":         args.Type,
		"out_trade_no": args.ServiceTradeNo,
		"notify_url":   args.NotifyUrl.String(),
		"name":         args.Name,
		"money":        args.Money,
		"device":       device,
		"sign_type":    "MD5",
		"return_url":   args.ReturnUrl.String(),
	}
	// clientip is required by mapi.php; include for submit.php as well so
	// gateways that enforce it on page-jump still work.
	if ip := NormalizeClientIP(args.ClientIP); ip != "" {
		params["clientip"] = ip
	}
	return epay.GenerateParams(params, operation_setting.EpayKey)
}

// EpayPurchase builds a signed EasyPay request.
// For mapi.php it performs a server-side API call and returns the payment URL.
// For submit.php it returns form action URL + signed fields for browser POST.
func EpayPurchase(args *EpayPurchaseArgs) (*EpayPurchaseResult, error) {
	if args == nil {
		return nil, fmt.Errorf("purchase args is nil")
	}
	if args.NotifyUrl == nil || args.ReturnUrl == nil {
		return nil, fmt.Errorf("notify_url and return_url are required")
	}
	if strings.TrimSpace(operation_setting.EpayId) == "" || strings.TrimSpace(operation_setting.EpayKey) == "" {
		return nil, fmt.Errorf("epay credentials not configured")
	}

	endpoint, mode, err := resolveEpayEndpoint(operation_setting.PayAddress)
	if err != nil {
		return nil, err
	}

	params := buildEpayPurchaseParams(args)
	if mode == epayModeMapi && strings.TrimSpace(params["clientip"]) == "" {
		return nil, fmt.Errorf("clientip is required for mapi.php payment")
	}

	if mode == epayModeMapi {
		launchURL, launchType, err := callEpayMapi(endpoint, params)
		if err != nil {
			return nil, err
		}
		// Direct launch — no form fields needed.
		return &EpayPurchaseResult{URL: launchURL, Params: map[string]string{}, Type: launchType}, nil
	}

	return &EpayPurchaseResult{URL: endpoint, Params: params, Type: EpayLaunchForm}, nil
}

type epayMapiResponse struct {
	Code      any    `json:"code"`
	Msg       string `json:"msg"`
	TradeNo   string `json:"trade_no"`
	PayURL    string `json:"payurl"`
	QRCode    string `json:"qrcode"`
	URLScheme string `json:"urlscheme"`
}

func callEpayMapi(endpoint string, params map[string]string) (string, EpayLaunchType, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	client := GetHttpClient()
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	// Use a dedicated timeout so a slow payment gateway does not hang forever
	// even when the shared relay client has no timeout configured.
	reqClient := *client
	if reqClient.Timeout == 0 || reqClient.Timeout > 30*time.Second {
		reqClient.Timeout = 30 * time.Second
	}

	resp, err := reqClient.PostForm(endpoint, form)
	if err != nil {
		return "", "", fmt.Errorf("mapi request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("read mapi response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("mapi http status %d: %s", resp.StatusCode, truncateForErr(string(body), 200))
	}

	var result epayMapiResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parse mapi response failed: %w, body=%s", err, truncateForErr(string(body), 200))
	}

	if !epayMapiCodeOK(result.Code) {
		msg := strings.TrimSpace(result.Msg)
		if msg == "" {
			msg = "unknown mapi error"
		}
		return "", "", fmt.Errorf("mapi business error: %s", msg)
	}

	// Prefer payurl, then qrcode, then urlscheme. Some gateways put Alipay/WeChat
	// scan links in payurl — reclassify those as qrcode so the frontend shows a
	// QR modal instead of opening render.alipay.com / weixin intermediate pages.
	if u := strings.TrimSpace(result.PayURL); u != "" {
		if looksLikePaymentQRContent(u) {
			return u, EpayLaunchQRCode, nil
		}
		return u, EpayLaunchURL, nil
	}
	if u := strings.TrimSpace(result.QRCode); u != "" {
		return u, EpayLaunchQRCode, nil
	}
	if u := strings.TrimSpace(result.URLScheme); u != "" {
		return u, EpayLaunchURLScheme, nil
	}
	return "", "", fmt.Errorf("mapi success but no payurl/qrcode/urlscheme returned")
}

// looksLikePaymentQRContent detects payment links that must be scanned as QR codes
// on desktop rather than opened in a browser tab.
//
// Opening https://qr.alipay.com/... in a desktop browser redirects to
// render.alipay.com scheme intermediate pages that try to wake the Alipay app
// and do not complete payment. Same for WeChat wxp/weixin schemes.
func looksLikePaymentQRContent(raw string) bool {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return false
	}
	switch {
	case strings.HasPrefix(s, "weixin://"),
		strings.HasPrefix(s, "wxp://"),
		strings.HasPrefix(s, "alipays://"),
		strings.HasPrefix(s, "alipay://"):
		return true
	case strings.Contains(s, "qr.alipay.com"),
		strings.Contains(s, "render.alipay.com"),
		strings.Contains(s, "qr.weixin.qq.com"),
		strings.Contains(s, "wx.tenpay.com"):
		return true
	default:
		return false
	}
}

func epayMapiCodeOK(code any) bool {
	switch v := code.(type) {
	case nil:
		return false
	case float64:
		return v == 1
	case int:
		return v == 1
	case int64:
		return v == 1
	case string:
		return v == "1" || strings.EqualFold(v, "success")
	default:
		// json.Number or other numeric encodings
		s := fmt.Sprint(v)
		return s == "1"
	}
}

func truncateForErr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// NormalizeClientIP returns a best-effort client IP string for payment gateways.
// Strips host:port form (e.g. from RemoteAddr) down to the IP when needed.
func NormalizeClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	// net.SplitHostPort needs brackets for IPv6; try common forms first.
	if host, _, err := splitHostPortSafe(ip); err == nil && host != "" {
		return host
	}
	return ip
}

func splitHostPortSafe(hostport string) (host, port string, err error) {
	// Accept "ip:port" and "[ipv6]:port"; plain IP returns error from SplitHostPort.
	return net.SplitHostPort(hostport)
}
