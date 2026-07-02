package appui

import (
	"time"

	setuptpl "github.com/HexmosTech/git-lrc/setup"
)

const (
	cloudAPIURL              = setuptpl.CloudAPIURL
	geminiKeysURL            = setuptpl.GeminiKeysURL
	defaultGeminiModel       = setuptpl.DefaultGeminiModel
	defaultGeminiHelperModel = setuptpl.DefaultGeminiHelperModel
	setupTimeout             = 5 * time.Minute
	issuesURL                = setuptpl.IssuesURL
)

type setupResult = setuptpl.SetupResult
type hexmosCallbackData = setuptpl.HexmosCallbackData
type ensureCloudUserRequest = setuptpl.EnsureCloudUserRequest
type ensureCloudUserResponse = setuptpl.EnsureCloudUserResponse
type createAPIKeyRequest = setuptpl.CreateAPIKeyRequest
type createAPIKeyResponse = setuptpl.CreateAPIKeyResponse
type validateKeyRequest = setuptpl.ValidateKeyRequest
type validateKeyResponse = setuptpl.ValidateKeyResponse
type createConnectorRequest = setuptpl.CreateConnectorRequest

var setupLandingPageTemplate = setuptpl.SetupLandingPageTemplate
var setupSuccessPageTemplate = setuptpl.SetupSuccessPageTemplate
var setupErrorPageTemplate = setuptpl.SetupErrorPageTemplate
