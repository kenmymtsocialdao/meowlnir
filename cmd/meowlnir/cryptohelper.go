package main

import (
	"context"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exerrors"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func CryptoHelperByBotUsername(ctx context.Context, eventProcessor *appservice.EventProcessor, as *appservice.AppService,
	cryptoStoreDB *dbutil.Database, idUserId id.UserID, pickleKey string,
	HandleMessage func(context.Context, *event.Event)) *cryptohelper.CryptoHelper {
	intentApi := as.Intent(idUserId)
	client := intentApi.Client
	client.SetAppServiceDeviceID = true
	cryptoStore := &crypto.SQLCryptoStore{
		DB:        cryptoStoreDB,
		AccountID: client.UserID.String(),
		PickleKey: []byte(pickleKey),
	}
	cryptoStore.InitFields()
	helper := exerrors.Must(cryptohelper.NewCryptoHelper(client, cryptoStore.PickleKey, cryptoStore))
	helper.DBAccountID = cryptoStore.AccountID
	helper.LoginAs = &mautrix.ReqLogin{
		Type: mautrix.AuthTypeAppservice,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: idUserId.Localpart(),
		},
		InitialDeviceDisplayName: "Meowlnir",
	}
	helper.Init(ctx)
	//eventProcessor.OnDeviceList(helper.Machine().HandleDeviceLists)
	//helper.CustomPostDecrypt = HandleMessage
	return helper
}
