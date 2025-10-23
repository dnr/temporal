package dynamicconfig

import (
	"regexp"
	"strings"

	"github.com/mitchellh/mapstructure"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/server/common/util"
)

var (
	MatchAnythingRE = regexp.MustCompile(".*")
	MatchNothingRE  = regexp.MustCompile(".^")
)

func ConvertWildcardStringListToRegexp(in any) (*regexp.Regexp, error) {
	// first convert raw value to list of strings
	var patterns []string
	if err := mapstructure.Decode(in, &patterns); err != nil {
		return nil, err
	}
	// then turn strings into regexp
	return util.WildCardStringsToRegexp(patterns)
}

func convertSdkWorkerOptions(in any) (sdkworker.Options, error) {
	// convert most fields
	var out sdkworker.Options
	if dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &out,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructureHookDuration,
		),
	}); err != nil {
		return out, err
	} else if err = dec.Decode(v); err != nil {
		return nil, err
	}

	// convert autoscaling behavior using custom struct
	var behavior struct {
		WorkflowTaskPollerBehavior pollerBehavior
		ActivityTaskPollerBehavior pollerBehavior
	}
	if err := mapstructure.Decode(v, &behavior); err != nil {
		return out, err
	}
	// put in worker options
	out.WorkflowTaskPollerBehavior = behavior.WorkflowTaskPollerBehavior.toPollerBehavior()
	out.ActivityTaskPollerBehavior = behavior.ActivityTaskPollerBehavior.toPollerBehavior()

	return out, nil
}

type pollerBehavior struct {
	Mode   string // "simple" or "auto"
	Simple sdkworker.PollerBehaviorSimpleMaximumOptions
	Auto   sdkworker.PollerBehaviorAutoscalingOptions
}

func (a pollerBehavior) toPollerBehavior() sdkworker.PollerBehavior {
	switch strings.ToLower(a.Mode) {
	case "simple":
		return sdkworker.NewPollerBehaviorSimpleMaximum(a.Simple)
	case "auto":
		return sdkworker.NewPollerBehaviorAutoscaling(a.Auto)
	default:
		return nil
	}
}
