package main

import (
	"context"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exerrors"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

func CryptoHelperByBotUsername(ctx context.Context, as *appservice.AppService, cryptoStoreDB *dbutil.Database, idUserId id.UserID, pickleKey string) *cryptohelper.CryptoHelper {
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
	//m.EventProcessor.OnDeviceList(helper.Machine().HandleDeviceLists)
	//helper.CustomPostDecrypt = m.HandleMessage
	return helper
}
