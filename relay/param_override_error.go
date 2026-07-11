package relay

import (
	relaycommon "github.com/elesos/tokenHub/relay/common"
	"github.com/elesos/tokenHub/types"
)

func newAPIErrorFromParamOverride(err error) *types.NewAPIError {
	if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
		return relaycommon.NewAPIErrorFromParamOverride(fixedErr)
	}
	return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
}
