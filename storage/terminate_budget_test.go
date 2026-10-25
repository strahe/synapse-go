package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type terminationBudgetTransport func(*http.Request) (*http.Response, error)

func (f terminationBudgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTerminateService_ProviderBudgetSpansSubmissionAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name            string
		providerTimeout time.Duration
		callerTimeout   time.Duration
	}{
		{name: "negative timeout without caller deadline", providerTimeout: -time.Second},
		{name: "negative timeout with caller deadline", providerTimeout: -time.Second, callerTimeout: time.Minute},
		{name: "shorter caller deadline", providerTimeout: time.Hour, callerTimeout: time.Minute},
		{name: "provider deadline", providerTimeout: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.callerTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.callerTimeout)
				defer cancel()
			}
			callerDeadline, callerHasDeadline := ctx.Deadline()
			var stages []string
			var operationDeadline time.Time
			var operationHasDeadline bool
			checkBudget := func(stage string, stageCtx context.Context) {
				t.Helper()
				deadline, hasDeadline := stageCtx.Deadline()
				if len(stages) == 0 {
					operationDeadline, operationHasDeadline = deadline, hasDeadline
					if callerHasDeadline && (!hasDeadline || !deadline.Equal(callerDeadline)) {
						t.Fatalf("submission deadline=%s present=%v, want caller deadline=%s", deadline, hasDeadline, callerDeadline)
					}
					if !callerHasDeadline && hasDeadline != (tc.providerTimeout > 0) {
						t.Fatalf("submission deadline present=%v with provider timeout=%s", hasDeadline, tc.providerTimeout)
					}
				} else if hasDeadline != operationHasDeadline || !deadline.Equal(operationDeadline) {
					t.Fatalf("%s deadline=%s present=%v, want submission deadline=%s present=%v", stage, deadline, hasDeadline, operationDeadline, operationHasDeadline)
				}
				stages = append(stages, stage)
			}
			s := terminationSubmitService(t, "https://termination.invalid")
			s.providerHTTPClient = &http.Client{Transport: terminationBudgetTransport(func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					defer func() { _ = req.Body.Close() }()
				}
				checkBudget(req.Method, req.Context())
				status := http.StatusNotFound
				if req.Method == http.MethodPost {
					status = http.StatusAccepted
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
			})}
			reads := 0
			s.dsReader = terminationRecoveryReader{read: func(readCtx context.Context, id types.BigInt) (*warmstorage.DataSetInfo, error) {
				reads++
				if reads == 1 {
					deadline, hasDeadline := readCtx.Deadline()
					if hasDeadline != callerHasDeadline || !deadline.Equal(callerDeadline) {
						t.Fatalf("preparation deadline=%s present=%v, want caller deadline=%s present=%v", deadline, hasDeadline, callerDeadline, callerHasDeadline)
					}
				} else {
					checkBudget("chain reconciliation", readCtx)
				}
				return &warmstorage.DataSetInfo{DataSetID: id, ProviderID: testProvider().ID, Payer: s.payerAddr}, nil
			}}
			result, err := s.TerminateService(ctx, types.NewBigInt(7), &TerminateServiceOptions{ProviderWaitTimeout: tc.providerTimeout})
			var notFound *pdp.WaitForTerminateServiceNotFoundError
			var staged *TerminateServiceError
			if result != nil || !errors.As(err, &notFound) || !errors.As(err, &staged) || staged.Stage != TerminateServiceStageWait || staged.Submission == nil || !staged.Submission.Accepted {
				t.Fatalf("result=%+v error=%v, want accepted submission with provider observation failure", result, err)
			}
			if reads != 2 || len(stages) != 3 || stages[0] != http.MethodPost || stages[1] != http.MethodGet || stages[2] != "chain reconciliation" {
				t.Fatalf("reads=%d stages=%v, want preparation then POST, GET, reconciliation", reads, stages)
			}
		})
	}
}
