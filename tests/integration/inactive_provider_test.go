//go:build integration

package integration_test

import (
	"context"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
)

func TestIntegration_InactiveProviderCDNDownload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := integrationtest.NewDefaultClient(t, ctx)
	dataSets, err := client.WarmStorage().GetClientDataSetsWithDetails(ctx, client.Address(), true)
	if err != nil {
		t.Fatal(err)
	}
	active := make(map[string]bool)
	for _, dataSet := range dataSets {
		if !dataSet.IsLive || !dataSet.WithCDN || !dataSet.HasActivePieces || dataSet.PDPEndEpoch != 0 {
			continue
		}
		key := idconv.Key(dataSet.ProviderID)
		providerActive, known := active[key]
		if !known {
			_, err := client.SPRegistry().GetPDPProvider(ctx, dataSet.ProviderID)
			if err != nil && !errors.Is(err, spregistry.ErrNotFound) {
				t.Fatal(err)
			}
			providerActive = err == nil
			active[key] = providerActive
		}
		if providerActive {
			continue
		}
		rpc, err := ethclient.DialContext(ctx, integrationtest.RPCURL())
		if err != nil {
			t.Fatal(err)
		}
		defer rpc.Close()
		verifier, err := pdpverifier.NewPDPVerifierCaller(client.ResolvedAddresses().PDPVerifier, rpc)
		if err != nil {
			t.Fatal(err)
		}
		page, err := verifier.GetActivePiecesByCursor(&bind.CallOpts{Context: ctx}, dataSet.DataSetID.Big(), new(big.Int), big.NewInt(1))
		if err != nil || len(page.Pieces) == 0 {
			t.Fatalf("inactive fixture pieces=%d error=%v", len(page.Pieces), err)
		}
		pieceCID, err := cid.Cast(page.Pieces[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		body, err := client.Storage().Download(ctx, pieceCID, &storage.DownloadOptions{DataSetID: &dataSet.DataSetID})
		if err != nil {
			t.Fatalf("inactive fixture dataset=%s provider=%s: %v", dataSet.DataSetID, dataSet.ProviderID, err)
		}
		bytesRead, readErr := io.Copy(io.Discard, body)
		_ = body.Close()
		if readErr != nil || bytesRead == 0 {
			t.Fatalf("inactive CDN bytes=%d error=%v", bytesRead, readErr)
		}
		t.Logf("inactive CDN fixture verified: dataset=%s provider=%s bytes=%d", dataSet.DataSetID, dataSet.ProviderID, bytesRead)
		return
	}
	t.Skip("no owned, live CDN data set with an inactive provider; this P0 scenario remains unverified")
}
