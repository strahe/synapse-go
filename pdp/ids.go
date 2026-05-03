package pdp

import (
	"encoding/json"
	"fmt"

	"github.com/strahe/synapse-go/types"
)

func parseBigIntNumber(op, name string, n json.Number) (types.BigInt, error) {
	id, err := types.ParseBigInt(n.String())
	if err != nil {
		return types.BigInt{}, fmt.Errorf("%s: bad %s %q", op, name, n)
	}
	return id, nil
}
