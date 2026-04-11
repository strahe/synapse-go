package typeddata

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

func benchSignHash(b *testing.B) (func([]byte) ([]byte, error), common.Address) {
	b.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return func(hash []byte) ([]byte, error) {
		return crypto.Sign(hash, key)
	}, addr
}

func benchDomain() apitypes.TypedDataDomain {
	return NewDomain(big.NewInt(314159), common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"))
}

func benchCID(b *testing.B, data []byte) cid.Cid {
	b.Helper()
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	if err != nil {
		b.Fatal(err)
	}
	return cid.NewCidV1(cid.Raw, hash)
}

func BenchmarkSignCreateDataSet(b *testing.B) {
	signHash, addr := benchSignHash(b)
	domain := benchDomain()
	dataSetID := big.NewInt(1)
	b.ResetTimer()
	for range b.N {
		_, err := SignCreateDataSet(signHash, domain, dataSetID, addr, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignAddPieces_1(b *testing.B) {
	benchmarkSignAddPieces(b, 1)
}

func BenchmarkSignAddPieces_10(b *testing.B) {
	benchmarkSignAddPieces(b, 10)
}

func benchmarkSignAddPieces(b *testing.B, n int) {
	b.Helper()
	signHash, _ := benchSignHash(b)
	domain := benchDomain()
	dataSetID := big.NewInt(1)
	nonce := big.NewInt(0)

	pieces := make([]cid.Cid, n)
	metadata := make([][]MetadataEntry, n)
	for i := range pieces {
		pieces[i] = benchCID(b, []byte{byte(i)})
	}

	b.ResetTimer()
	for range b.N {
		_, err := SignAddPieces(signHash, domain, dataSetID, nonce, pieces, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}
