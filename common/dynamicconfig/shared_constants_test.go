// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dynamicconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestEnsureRetryPolicyDefaults(t *testing.T) {
	defaultRetrySettings := DefaultRetrySettings{
		InitialInterval:            time.Second,
		MaximumIntervalCoefficient: 100,
		BackoffCoefficient:         2.0,
		MaximumAttempts:            120,
	}

	defaultRetryPolicy := &commonpb.RetryPolicy{
		InitialInterval:    durationpb.New(1 * time.Second),
		MaximumInterval:    durationpb.New(100 * time.Second),
		BackoffCoefficient: 2.0,
		MaximumAttempts:    120,
	}

	testCases := []struct {
		name  string
		input *commonpb.RetryPolicy
		want  *commonpb.RetryPolicy
	}{
		{
			name:  "default fields are set ",
			input: &commonpb.RetryPolicy{},
			want:  defaultRetryPolicy,
		},
		{
			name: "non-default InitialIntervalInSeconds is not set",
			input: &commonpb.RetryPolicy{
				InitialInterval: durationpb.New(2 * time.Second),
			},
			want: &commonpb.RetryPolicy{
				InitialInterval:    durationpb.New(2 * time.Second),
				MaximumInterval:    durationpb.New(200 * time.Second),
				BackoffCoefficient: 2,
				MaximumAttempts:    120,
			},
		},
		{
			name: "non-default MaximumIntervalInSeconds is not set",
			input: &commonpb.RetryPolicy{
				MaximumInterval: durationpb.New(1000 * time.Second),
			},
			want: &commonpb.RetryPolicy{
				InitialInterval:    durationpb.New(1 * time.Second),
				MaximumInterval:    durationpb.New(1000 * time.Second),
				BackoffCoefficient: 2,
				MaximumAttempts:    120,
			},
		},
		{
			name: "non-default BackoffCoefficient is not set",
			input: &commonpb.RetryPolicy{
				BackoffCoefficient: 1.5,
			},
			want: &commonpb.RetryPolicy{
				InitialInterval:    durationpb.New(1 * time.Second),
				MaximumInterval:    durationpb.New(100 * time.Second),
				BackoffCoefficient: 1.5,
				MaximumAttempts:    120,
			},
		},
		{
			name: "non-default Maximum attempts is not set",
			input: &commonpb.RetryPolicy{
				MaximumAttempts: 49,
			},
			want: &commonpb.RetryPolicy{
				InitialInterval:    durationpb.New(1 * time.Second),
				MaximumInterval:    durationpb.New(100 * time.Second),
				BackoffCoefficient: 2,
				MaximumAttempts:    49,
			},
		},
		{
			name: "non-retryable errors are set",
			input: &commonpb.RetryPolicy{
				NonRetryableErrorTypes: []string{"testFailureType"},
			},
			want: &commonpb.RetryPolicy{
				InitialInterval:        durationpb.New(1 * time.Second),
				MaximumInterval:        durationpb.New(100 * time.Second),
				BackoffCoefficient:     2.0,
				MaximumAttempts:        120,
				NonRetryableErrorTypes: []string{"testFailureType"},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			EnsureRetryPolicyDefaults(tt.input, defaultRetrySettings)
			assert.Equal(t, tt.want, tt.input)
		})
	}
}

func Test_FromConfigToRetryPolicy(t *testing.T) {
	options := map[string]interface{}{
		initialIntervalInSecondsConfigKey:   2,
		maximumIntervalCoefficientConfigKey: 100.0,
		backoffCoefficientConfigKey:         4.0,
		maximumAttemptsConfigKey:            5,
	}

	defaultSettings := FromConfigToDefaultRetrySettings(options)
	assert.Equal(t, 2*time.Second, defaultSettings.InitialInterval)
	assert.Equal(t, 100.0, defaultSettings.MaximumIntervalCoefficient)
	assert.Equal(t, 4.0, defaultSettings.BackoffCoefficient)
	assert.Equal(t, int32(5), defaultSettings.MaximumAttempts)
}
