//go:build !consulent
// +build !consulent

package xds

import (
	"fmt"
)

func (ft discoChainTargets) orderByLocality() ([]discoChainTargetGroup, error) {
	return nil, fmt.Errorf("order-by-locality is a Consul Enterprise feature")
}
