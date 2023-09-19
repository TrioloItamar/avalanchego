// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package transfer

import (
	"context"
	"log"
	"time"

	"github.com/spf13/cobra"

	"github.com/ava-labs/avalanchego/vms/example/xsvm/api"
	"github.com/ava-labs/avalanchego/vms/example/xsvm/tx"
)

func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "transfer",
		Short: "Issues a transfer transaction",
		RunE:  transferFunc,
	}
	flags := c.Flags()
	AddFlags(flags)
	return c
}

func transferFunc(c *cobra.Command, args []string) error {
	flags := c.Flags()
	config, err := ParseFlags(flags, args)
	if err != nil {
		return err
	}

	txStatus, err := Transfer(c.Context(), config)
	if err != nil {
		return err
	}

	msg, err := txStatus.GetMessage()
	if err != nil {
		return err
	}
	log.Print(msg)

	return nil
}

func Transfer(ctx context.Context, config *Config) (*tx.TxIssueStatus, error) {
	client := api.NewClient(config.URI, config.ChainID.String())

	nonce, err := client.Nonce(ctx, config.PrivateKey.Address())
	if err != nil {
		return nil, err
	}

	utx := &tx.Transfer{
		ChainID: config.ChainID,
		Nonce:   nonce,
		MaxFee:  config.MaxFee,
		AssetID: config.AssetID,
		Amount:  config.Amount,
		To:      config.To,
	}
	stx, err := tx.Sign(utx, config.PrivateKey)
	if err != nil {
		return nil, err
	}

	issueTxStartTime := time.Now()
	txID, err := client.IssueTx(ctx, stx)
	if err != nil {
		return nil, err
	}

	return &tx.TxIssueStatus{
		Tx:        stx,
		TxID:      txID,
		StartTime: issueTxStartTime,
	}, nil
}