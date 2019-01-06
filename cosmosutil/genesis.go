package cosmosutil

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"

	app "github.com/cosmos/cosmos-sdk/cmd/gaia/app"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

type account struct {
	Address sdk.AccAddress
	Coins   sdk.Coins
}

func loadGenesis(cdc *codec.Codec, path string) (*tmtypes.GenesisDoc, *app.GenesisState, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var genDoc tmtypes.GenesisDoc
	if err = cdc.UnmarshalJSON(data, &genDoc); err != nil {
		return nil, nil, err
	}

	var appState app.GenesisState
	if err = cdc.UnmarshalJSON(genDoc.AppState, &appState); err != nil {
		return nil, nil, err
	}

	return &genDoc, &appState, nil
}

func addValidator(validators []tmtypes.GenesisValidator, pubKey string) ([]tmtypes.GenesisValidator, error) {
	pKey, err := sdk.GetValPubKeyBech32(pubKey)
	if err != nil {
		return nil, err
	}

	// NOTE: the validator voting power is hardcoded to 10
	validator := tmtypes.GenesisValidator{
		Address: pKey.Address(),
		PubKey:  pKey,
		Power:   10,
		Name:    "",
	}
	return append(validators, validator), nil
}

func addAccount(accounts []account, address, coins string) ([]account, error) {
	addr, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		return nil, err
	}

	coinList, err := sdk.ParseCoins(coins)
	if err != nil {
		return nil, err
	}
	coinList.Sort()

	acc := account{
		Address: addr,
		Coins:   coinList,
	}
	return append(accounts, acc), nil
}

// GenesisAdd adds accounts or validator to a genesis file
func GenesisAdd(path string, adds []string) error {
	formatError := errors.New("formatting error: --genesis-add validator|account:address(,coins,...) - " +
		"example: --genesis-add account:1234abcde,10mycoin --genesis-add validator:abcde1234")

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrap(err, "cannot read genesis file")
	}

	cdc := codec.New()
	codec.RegisterCrypto(cdc)
	genesis := map[string]interface{}{}
	if err := json.Unmarshal([]byte(data), &genesis); err != nil {
		return err
	}
	appState := genesis["app_state"].(map[string]interface{})

	accounts := appState["accounts"].([]account)
	validators := genesis["validators"].([]tmtypes.GenesisValidator)
	for _, add := range adds {
		sadd := strings.SplitN(add, ":", 2)
		if len(sadd) < 2 {
			return formatError
		}
		switch key := sadd[0]; key {
		case "validator":
			validators, err = addValidator(validators, sadd[1])
			if err != nil {
				return errors.Wrap(err, "cannot add validator")
			}
		case "account":
			acc := strings.SplitN(sadd[1], ",", 2)
			coins := ""
			if len(acc) > 1 {
				coins = acc[1]
			}
			accounts, err = addAccount(accounts, acc[0], coins)
			if err != nil {
				return errors.Wrap(err, "cannot add account")
			}
		default:
			return formatError
		}
	}

	genesis["validators"] = validators
	appState["accounts"] = accounts
	genesis["app_state"] = appState
	genesisJSON, err := json.Marshal(genesis)
	if err != nil {
		return errors.Wrap(err, "cannot generate new genesis file")
	}
	if err := ioutil.WriteFile("/tmp/genesis.json", genesisJSON, 0644); err != nil {
		return errors.Wrap(err, "cannot write new genesis file")
	}

	return nil
}
