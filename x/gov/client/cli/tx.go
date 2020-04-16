package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	sdkGovCli "github.com/cosmos/cosmos-sdk/x/gov/client/cli"
	"github.com/spf13/cobra"

	"github.com/okex/okchain/x/gov/types"
)

// Proposal flags
const (
	flagTitle        = "title"
	flagDescription  = "description"
	flagProposalType = "type"
	flagDeposit      = "deposit"
	flagProposal     = "proposal"
)

type proposal struct {
	Title        string
	Description  string
	ProposalType string
	Deposit      string
}

// proposalFlags defines the core required fields of a proposal. It is used to
// verify that these values are not provided in conjunction with a JSON proposal
// file.
var proposalFlags = []string{
	flagTitle,
	flagDescription,
	flagProposalType,
	flagDeposit,
}

// GetTxCmd returns the transaction commands for this module
// governance ModuleClient is slightly different from other ModuleClients in that
// it contains a slice of "proposal" child commands. These commands are respective
// to proposal type handlers that are implemented in other modules but are mounted
// under the governance CLI (eg. parameter change proposals).
func GetTxCmd(storeKey string, cdc *codec.Codec, pcmds []*cobra.Command) *cobra.Command {
	govTxCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Governance transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmdSubmitProp := getCmdSubmitProposal(cdc)
	for _, pcmd := range pcmds {
		cmdSubmitProp.AddCommand(client.PostCommands(pcmd)[0])
	}

	govTxCmd.AddCommand(client.PostCommands(
		getCmdDeposit(cdc),
		sdkGovCli.GetCmdVote(cdc),
		cmdSubmitProp,
	)...)

	return govTxCmd
}

// getCmdSubmitProposal implements submitting a proposal transaction command.
func getCmdSubmitProposal(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-proposal",
		Short: "Submit a proposal along with an initial deposit",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Submit a proposal along with an initial deposit.
Proposal title, description, type and deposit can be given directly or through a proposal JSON file.

Example:
$ %s tx gov submit-proposal --proposal="path/to/proposal.json" --from mykey

Where proposal.json contains:

{
  "title": "Test Proposal",
  "description": "My awesome proposal",
  "type": "Text",
  "deposit": "10%s"
}

Which is equivalent to:

$ %s tx gov submit-proposal --title="Test Proposal" --description="My awesome proposal" --type="Text" \
	--deposit="10%s" --from mykey
`,
				version.ClientName, sdk.DefaultBondDenom, version.ClientName, sdk.DefaultBondDenom,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			txBldr := auth.NewTxBuilderFromCLI().WithTxEncoder(utils.GetTxEncoder(cdc))
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			proposal, err := parseSubmitProposalFlags()
			if err != nil {
				return err
			}

			amount, err := sdk.ParseDecCoins(proposal.Deposit)
			if err != nil {
				return err
			}

			content := types.ContentFromProposalType(proposal.Title, proposal.Description, proposal.ProposalType)

			msg := types.NewMsgSubmitProposal(content, amount, cliCtx.GetFromAddress())
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(cliCtx, txBldr, []sdk.Msg{msg})
		},
	}

	cmd.Flags().String(flagTitle, "", "title of proposal")
	cmd.Flags().String(flagDescription, "", "description of proposal")
	cmd.Flags().String(flagProposalType, "",
		"proposalType of proposal, types: text/parameter_change/software_upgrade")
	cmd.Flags().String(flagDeposit, "", "deposit of proposal")
	cmd.Flags().String(flagProposal, "",
		"proposal file path (if this path is given, other proposal flags are ignored)")

	return cmd
}

// getCmdDeposit implements depositing tokens for an active proposal.
func getCmdDeposit(cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "deposit [proposal-id] [deposit]",
		Args:  cobra.ExactArgs(2),
		Short: "deposit tokens for an active proposal",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Submit a deposit for an active proposal. You can
find the proposal-id by running "%s query gov proposals".

Example:
$ %s tx gov deposit 1 10okt --from mykey
`,
				version.ClientName, version.ClientName,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			txBldr := auth.NewTxBuilderFromCLI().WithTxEncoder(utils.GetTxEncoder(cdc))
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			// validate that the proposal id is a uint
			proposalID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("proposal-id %s not a valid uint, please input a valid proposal-id", args[0])
			}

			// Get depositor address
			from := cliCtx.GetFromAddress()

			// Get amount of coins
			amount, err := sdk.ParseDecCoins(args[1])
			if err != nil {
				return err
			}

			msg := types.NewMsgDeposit(from, proposalID, amount)
			err = msg.ValidateBasic()
			if err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(cliCtx, txBldr, []sdk.Msg{msg})
		},
	}
}

// DONTCOVER
