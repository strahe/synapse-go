package filbeam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

// Service is a client for the FilBeam stats and retrieval APIs.
// It is safe for concurrent use.
type Service struct {
	baseURL         string
	retrievalDomain string
	httpClient      *http.Client
	logger          *slog.Logger
	lifecycle       *lifecycle.Lifecycle
}

// Options configures a [Service].
type Options struct {
	// Chain selects the FilBeam environment. Only chain.Mainnet and
	// chain.Calibration are supported; any other value causes New to
	// return an error wrapping chain.ErrUnknownChain.
	// Zero value is chain.Mainnet.
	Chain chain.Chain

	// HTTPClient is the HTTP client used for API requests.
	// If nil, http.DefaultClient is used.
	HTTPClient *http.Client

	// RetrievalDomain overrides the chain default FilBeam retrieval domain.
	// Leave empty for the built-in Mainnet / Calibration defaults.
	RetrievalDomain string

	// Logger is the structured logger. If nil, logging is silent.
	Logger *slog.Logger

	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// ErrClosed. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle
}

// New creates a [Service] for the given chain.
// Returns an error wrapping [chain.ErrUnknownChain] if opts.Chain is not a
// supported FilBeam network.
func New(opts Options) (*Service, error) {
	endpoints, ok := filbeamEndpointsForChain(opts.Chain)
	if !ok {
		return nil, fmt.Errorf("filbeam.New: %w: %v", chain.ErrUnknownChain, opts.Chain)
	}
	if opts.RetrievalDomain != "" {
		domain, err := normalizeRetrievalDomain(opts.RetrievalDomain)
		if err != nil {
			return nil, fmt.Errorf("filbeam.New: %w: invalid retrieval domain: %w", ErrInvalidArgument, err)
		}
		endpoints.retrievalDomain = domain
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Service{
		baseURL:         endpoints.statsBaseURL,
		retrievalDomain: endpoints.retrievalDomain,
		httpClient:      httpClient,
		logger:          opts.Logger,
		lifecycle:       opts.Lifecycle,
	}, nil
}

type filbeamEndpoints struct {
	statsBaseURL    string
	retrievalDomain string
}

// filbeamEndpointsForChain returns the FilBeam endpoints for the given chain.
func filbeamEndpointsForChain(c chain.Chain) (filbeamEndpoints, bool) {
	switch c {
	case chain.Mainnet:
		return filbeamEndpoints{
			statsBaseURL:    "https://stats.filbeam.com",
			retrievalDomain: "filbeam.io",
		}, true
	case chain.Calibration:
		return filbeamEndpoints{
			statsBaseURL:    "https://calibration.stats.filbeam.com",
			retrievalDomain: "calibration.filbeam.io",
		}, true
	default:
		return filbeamEndpoints{}, false
	}
}

func normalizeRetrievalDomain(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", errors.New("empty")
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/?#@") {
		return "", errors.New("must be a host name, not a URL")
	}
	return domain, nil
}

// Retriever downloads pieces through FilBeam for one owner address.
// It is safe for concurrent use.
type Retriever struct {
	service *Service
	owner   common.Address
}

// NewRetriever creates a FilBeam retriever scoped to owner.
func (s *Service) NewRetriever(owner common.Address) (*Retriever, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if owner == (common.Address{}) {
		return nil, fmt.Errorf("filbeam.NewRetriever: %w: zero owner", ErrInvalidArgument)
	}
	return &Retriever{service: s, owner: owner}, nil
}

// DownloadPiece downloads a PieceCID through FilBeam.
func (r *Retriever) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
	if r == nil || r.service == nil {
		return nil, ErrUninitialized
	}
	if err := r.service.checkInit(); err != nil {
		return nil, err
	}
	if err := validatePieceCID(pieceCID); err != nil {
		return nil, fmt.Errorf("filbeam.Retriever.DownloadPiece: %w: %w", ErrInvalidArgument, err)
	}
	rawURL := fmt.Sprintf("https://%s.%s/%s", strings.ToLower(r.owner.Hex()), r.service.retrievalDomain, pieceCID.String())
	if err := r.headPiece(ctx, rawURL); err != nil {
		return nil, err
	}
	return r.getPiece(ctx, rawURL)
}

func validatePieceCID(c cid.Cid) error {
	if !c.Defined() {
		return errors.New("undefined pieceCID")
	}
	if piece.Validate(c) == nil {
		return nil
	}
	if _, err := piece.ParseV2(c); err == nil {
		return nil
	}
	return fmt.Errorf("not a piece CID: %s", c)
}

func (r *Retriever) headPiece(ctx context.Context, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return fmt.Errorf("filbeam.Retriever.DownloadPiece: build HEAD request: %w", err)
	}
	resp, err := r.service.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("filbeam.Retriever.DownloadPiece: HEAD: %w", err)
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("filbeam.Retriever.DownloadPiece: HEAD %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return nil
}

func (r *Retriever) getPiece(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("filbeam.Retriever.DownloadPiece: build GET request: %w", err)
	}
	resp, err := r.service.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filbeam.Retriever.DownloadPiece: GET: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("filbeam.Retriever.DownloadPiece: GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if resp.Body == nil {
		return nil, errors.New("filbeam.Retriever.DownloadPiece: GET returned nil body")
	}
	return resp.Body, nil
}

// statsResponse mirrors the JSON returned by the FilBeam stats API.
// The quota fields are string-encoded integers per the API contract.
type statsResponse struct {
	CDNEgressQuota       string `json:"cdnEgressQuota"`
	CacheMissEgressQuota string `json:"cacheMissEgressQuota"`
}

// GetDataSetStats fetches remaining egress quotas for a FWSS data set.
// Returns ErrDataSetNotFound when the data set does not exist on FilBeam.
//
// Transient failures (most transport errors and HTTP 5xx) are retried with
// jittered exponential backoff. Errors matching
// [context.Canceled] or [context.DeadlineExceeded] are returned immediately;
// non-transient statuses (4xx other than 404) and decode errors are also
// returned without retry.
func (s *Service) GetDataSetStats(ctx context.Context, dataSetID types.BigInt) (*DataSetStats, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w", ErrInvalidArgument)
	}
	url := fmt.Sprintf("%s/data-set/%s", s.baseURL, dataSetID.String())

	stats, err := retry.Do(ctx, func(ctx context.Context) (*DataSetStats, error) {
		return s.fetchStats(ctx, url, dataSetID)
	}, retry.WithRetryIf(isTransientFilbeamErr))
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// errTransientFilbeam marks fetchStats errors that should be retried. It is
// never returned to callers.
var errTransientFilbeam = errors.New("filbeam: transient")

func isTransientFilbeamErr(err error) bool { return errors.Is(err, errTransientFilbeam) }

func (s *Service) fetchStats(ctx context.Context, url string, dataSetID types.BigInt) (*DataSetStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: http: %w: %w", errTransientFilbeam, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w: id=%s", ErrDataSetNotFound, dataSetID.String())
	}
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: HTTP %d: %s: %w", resp.StatusCode, string(body), errTransientFilbeam)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw statsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: decode: %w", err)
	}

	cdnQuota, ok := new(big.Int).SetString(raw.CDNEgressQuota, 10)
	if !ok {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: invalid cdnEgressQuota: %q", raw.CDNEgressQuota)
	}
	cacheMissQuota, ok := new(big.Int).SetString(raw.CacheMissEgressQuota, 10)
	if !ok {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: invalid cacheMissEgressQuota: %q", raw.CacheMissEgressQuota)
	}

	return &DataSetStats{
		CDNEgressQuota:       cdnQuota,
		CacheMissEgressQuota: cacheMissQuota,
	}, nil
}
