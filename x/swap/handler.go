package swap

import (
	"fmt"
	"github.com/okex/okchain/x/common"
	"github.com/okex/okchain/x/common/perf"
	"github.com/okex/okchain/x/swap/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewHandler creates an sdk.Handler for all the swap type messages
func NewHandler(k Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		ctx = ctx.WithEventManager(sdk.NewEventManager())
		var handlerFun func() sdk.Result
		var name string
		switch msg := msg.(type) {
		case types.MsgAddLiquidity:
			name = "handleMsgAddLiquidity"
			handlerFun = func() sdk.Result {
				return handleMsgAddLiquidity(ctx, k, msg)
			}
		case types.MsgCreateExchange:
			name = "handleMsgCreateExchange"
			handlerFun = func() sdk.Result {
				return handleMsgCreateExchange(ctx, k, msg)
			}
		default:
			errMsg := fmt.Sprintf("Invalid msg type: %v", msg.Type())
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
		seq := perf.GetPerf().OnDeliverTxEnter(ctx, types.ModuleName, name)
		defer perf.GetPerf().OnDeliverTxExit(ctx, types.ModuleName, name, seq)
		return handlerFun()
	}
}

func handleMsgCreateExchange(ctx sdk.Context, k Keeper, msg types.MsgCreateExchange) sdk.Result {
	event := sdk.NewEvent(sdk.EventTypeMessage, sdk.NewAttribute(sdk.AttributeKeyModule, types.ModuleName))
	err:=k.IsTokenExits(ctx,msg.Token)
	if err != nil{
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  err.Error(),
		}
	}

	//TODO keys with prefix byte 0x01
	tokenPair := msg.Token + "_" + common.NativeToken

	swapTokenPair, err := k.GetSwapTokenPair(ctx, tokenPair)
	if err == nil {
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  "Failed to create Exchange: exchange is exit",
		}
	}

	poolName := "OIP3-"+msg.Token
	baseToken := sdk.NewDecCoinFromDec(msg.Token, sdk.ZeroDec())
	quoteToken := sdk.NewDecCoinFromDec(common.NativeToken, sdk.ZeroDec())
	poolToken := k.GetPoolTokenInfo(ctx, poolName)

	if len(poolToken.Symbol) == 0 {
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  "Failed to create Exchange: Pool Token not exit",
		}
	}

	swapTokenPair.BasePooledCoin = baseToken
	swapTokenPair.QuotePooledCoin = quoteToken
	swapTokenPair.PoolTokenName = poolName

	k.SetSwapTokenPair(ctx,tokenPair,swapTokenPair)


	event.AppendAttributes(sdk.NewAttribute("tokenpair", tokenPair))
	ctx.EventManager().EmitEvent(event)
	return sdk.Result{Events: ctx.EventManager().Events()}
}


func handleMsgAddLiquidity(ctx sdk.Context, k Keeper, msg types.MsgAddLiquidity) sdk.Result {
	event := sdk.NewEvent(sdk.EventTypeMessage, sdk.NewAttribute(sdk.AttributeKeyModule, types.ModuleName))

	if msg.Deadline < ctx.BlockTime().Unix() {
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  "blockTime exceeded deadline",
		}
	}
	swapTokenPair, err := k.GetSwapTokenPair(ctx, msg.GetSwapTokenPair())
	if err != nil {
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  err.Error(),
		}
	}
	baseTokens := sdk.NewDecCoinFromDec(msg.MaxBaseTokens.Denom, sdk.ZeroDec())
	var liquidity sdk.Dec
	poolToken := k.GetPoolTokenInfo(ctx, swapTokenPair.PoolTokenName)
	if swapTokenPair.QuotePooledCoin.Amount.IsZero() && swapTokenPair.BasePooledCoin.Amount.IsZero() {
		baseTokens.Amount = msg.MaxBaseTokens.Amount
		liquidity = sdk.NewDec(1)
	} else if swapTokenPair.BasePooledCoin.IsPositive() && swapTokenPair.QuotePooledCoin.IsPositive() {
		baseTokens.Amount = msg.QuoteTokens.Amount.Mul(swapTokenPair.BasePooledCoin.Amount).Quo(swapTokenPair.QuotePooledCoin.Amount)
		if poolToken.TotalSupply.IsZero() {
			return sdk.Result{
				Code: sdk.CodeInternal,
				Log:  fmt.Sprintf("unexpected totalSupply in poolToken %s", poolToken.String()),
			}
		}
		liquidity = msg.QuoteTokens.Amount.Quo(swapTokenPair.QuotePooledCoin.Amount).Mul(poolToken.TotalSupply)
	} else {
		return sdk.Result{
			Code: sdk.CodeInternal,
			Log:  fmt.Sprintf("invalid swapTokenPair %s", swapTokenPair.String()),
		}
	}
	if baseTokens.Amount.GT(msg.MaxBaseTokens.Amount) {
		return sdk.Result{
			Code:sdk.CodeInternal,
			Log: fmt.Sprintf("MaxBaseTokens is too high"),
		}
	}
	if liquidity.LT(msg.MinLiquidity) {
		return sdk.Result{
			Code:sdk.CodeInternal,
			Log: fmt.Sprintf("MinLiquidity is too low"),
		}
	}

	// transfer coins
	coins := sdk.DecCoins{
		msg.QuoteTokens,
		baseTokens,
	}
	err = k.SendCoinsToPool(ctx, coins, msg.Sender)
	if err != nil {
		return sdk.Result{
			Code:sdk.CodeInsufficientCoins,
			Log: fmt.Sprintf("insufficient Coins"),
		}
	}
	// update swapTokenPair
	swapTokenPair.QuotePooledCoin.Add(msg.QuoteTokens)
	swapTokenPair.BasePooledCoin.Add(baseTokens)
	k.SetSwapTokenPair(ctx, msg.GetSwapTokenPair(), swapTokenPair)

	// update poolToken
	poolCoins := sdk.NewDecCoinFromDec(poolToken.Symbol, liquidity)
	err = k.MintPoolCoinsToUser(ctx, sdk.DecCoins{poolCoins}, msg.Sender)
	if err != nil {
		return sdk.Result{
			Code:sdk.CodeInternal,
			Log: fmt.Sprintf("fail to mint poolCoins"),
		}
	}

	event.AppendAttributes(sdk.NewAttribute("liquidity", liquidity.String()))
	event.AppendAttributes(sdk.NewAttribute("baseTokens", baseTokens.String()))
	ctx.EventManager().EmitEvent(event)
	return sdk.Result{Events: ctx.EventManager().Events()}
}