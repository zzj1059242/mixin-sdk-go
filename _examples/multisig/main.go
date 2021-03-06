package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/fox-one/mixin-sdk-go"
	"github.com/gofrs/uuid"
	"github.com/shopspring/decimal"
)

var (
	config = flag.String("config", "", "keystore file path")
	pin    = flag.String("pin", "", "pin")

	ctx = context.Background()
)

func main() {
	flag.Parse()

	f, err := os.Open(*config)
	if err != nil {
		log.Panicln(err)
	}

	var store mixin.Keystore
	if err := json.NewDecoder(f).Decode(&store); err != nil {
		log.Panicln(err)
	}

	client, err := mixin.NewFromKeystore(&store)
	if err != nil {
		log.Panicln(err)
	}

	me, err := client.UserMe(ctx)
	if err != nil {
		log.Panicln(err)
	}

	// create sub wallet
	privateKey, _ := rsa.GenerateKey(rand.Reader, 1024)
	sub, subStore, err := client.CreateUser(ctx, privateKey, "sub user")
	if err != nil {
		log.Printf("CreateUser: %v", err)
		return
	}

	log.Println("create sub user", sub.UserID)

	subClient, _ := mixin.NewFromKeystore(subStore)
	if err := subClient.ModifyPin(ctx, "", *pin); err != nil {
		log.Printf("CreateUser: %v", err)
		return
	}

	members := []string{subClient.ClientID, client.ClientID, me.App.CreatorID}
	var threshold uint8 = 2

	go sumbitTransactionLoop(ctx, client, members, threshold)

	h, err := client.Transaction(ctx, &mixin.TransferInput{
		AssetID: "965e5c6e-434c-3fa9-b780-c50f43cd955c",
		Amount:  decimal.NewFromFloat(1),
		TraceID: uuid.Must(uuid.NewV4()).String(),
		Memo:    "send to multisig",
		OpponentMultisig: struct {
			Receivers []string `json:"receivers,omitempty"`
			Threshold uint8    `json:"threshold,omitempty"`
		}{
			Receivers: members,
			Threshold: threshold,
		},
	}, *pin)
	if err != nil {
		log.Panicln(err)
	}
	time.Sleep(time.Second * 5)

	var (
		utxo   *mixin.MultisigUTXO
		offset time.Time
	)
	const limit = 10
	for utxo == nil {
		outputs, err := client.ReadMultisigOutputs(ctx, members, threshold, offset, limit)
		if err != nil {
			log.Panicf("ReadMultisigOutputs: %v", err)
		}

		for _, output := range outputs {
			offset = output.UpdatedAt
			if hex.EncodeToString(output.TransactionHash[:]) == h.TransactionHash {
				utxo = output
				break
			}
		}
		if len(outputs) < limit {
			break
		}
	}

	if utxo == nil {
		log.Panicln("No Unspent UTXO")
	}

	amount := utxo.Amount.Div(decimal.NewFromFloat(2)).Truncate(8)
	if amount.IsZero() {
		amount = utxo.Amount
	}

	tx, err := client.MakeMultisigTransaction(ctx, &mixin.TransactionInput{
		Memo:   "multisig test",
		Inputs: []*mixin.MultisigUTXO{utxo},
		Outputs: []struct {
			Receivers []string
			Threshold uint8
			Amount    decimal.Decimal
		}{
			{
				[]string{client.ClientID},
				1,
				amount,
			},
		},
	})

	if err != nil {
		log.Panicf("MakeMultisigTransaction: %v", err)
	}

	raw, err := tx.DumpTransactionPayload()
	if err != nil {
		log.Panicf("DumpTransaction: %v", err)
	}

	{
		req, err := client.CreateMultisig(ctx, mixin.MultisigActionSign, raw)
		if err != nil {
			log.Panicf("CreateMultisig: sign %v", err)
		}

		req, err = client.SignMultisig(ctx, req.RequestID, *pin)
		if err != nil {
			log.Panicf("CreateMultisig: %v", err)
		}

		req, err = subClient.CreateMultisig(ctx, mixin.MultisigActionUnlock, raw)
		if err != nil {
			log.Panicf("CreateMultisig: sign %v", err)
		}

		err = subClient.CancelMultisig(ctx, req.RequestID)
		if err != nil {
			log.Panicf("CancelMultisig: %v", err)
		}
	}

	{
		req, err := client.CreateMultisig(ctx, mixin.MultisigActionSign, raw)
		if err != nil {
			log.Panicf("CreateMultisig: sign %v", err)
		}

		req, err = client.SignMultisig(ctx, req.RequestID, *pin)
		if err != nil {
			log.Panicf("CreateMultisig: %v", err)
		}

		if len(req.Signers) < int(req.Threshold) {
			req, err = client.CreateMultisig(ctx, mixin.MultisigActionUnlock, raw)
			if err != nil {
				log.Panicf("CreateMultisig: unlock %v", err)
			}

			err = client.UnlockMultisig(ctx, req.RequestID, *pin)
			if err != nil {
				log.Panicf("UnlockMultisig: %v", err)
			}
		}
	}

	{
		req, err := client.CreateMultisig(ctx, mixin.MultisigActionSign, raw)
		if err != nil {
			log.Panicf("CreateMultisig: sign %v", err)
		}

		req, err = client.SignMultisig(ctx, req.RequestID, *pin)
		if err != nil {
			log.Panicf("CreateMultisig: %v", err)
		}

		req, err = subClient.CreateMultisig(ctx, mixin.MultisigActionSign, raw)
		if err != nil {
			log.Panicf("CreateMultisig: sign %v", err)
		}

		req, err = subClient.SignMultisig(ctx, req.RequestID, *pin)
		if err != nil {
			log.Panicf("CreateMultisig: %v", err)
		}
	}
	time.Sleep(time.Second * 10)
}

func sumbitTransactionLoop(ctx context.Context, client *mixin.Client, members []string, threshold uint8) {
	const (
		limit = 100
	)

	var (
		sleepDur = time.Second
	)

	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(sleepDur):
			outputs, err := client.ReadMultisigOutputs(ctx, members, threshold, time.Time{}, limit)
			if err != nil {
				log.Panicf("ReadMultisigOutputs: %v", err)
			}

			for _, output := range outputs {
				if output.State == mixin.UTXOStateSigned && output.SignedBy != "" {
					tx, err := mixin.SendRawTransaction(ctx, output.SignedTx)
					if err != nil {
						log.Printf("ReadMultisigOutputs: %v\n", err)
						continue
					}
					log.Printf("submit transaction: %v", tx.Hash)
				}
			}
			sleepDur = time.Second
		}
	}
}
