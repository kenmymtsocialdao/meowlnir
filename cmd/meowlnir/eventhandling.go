package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/meowlnir/config"
)

func (m *Meowlnir) AddEventHandlers() {
	// Crypto stuff
	m.EventProcessor.OnOTK(m.HandleOTKCounts)
	m.EventProcessor.On(event.ToDeviceEncrypted, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceRoomKeyRequest, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceRoomKeyWithheld, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceBeeperRoomKeyAck, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceOrgMatrixRoomKeyWithheld, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationRequest, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationStart, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationAccept, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationKey, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationMAC, m.HandleToDeviceEvent)
	m.EventProcessor.On(event.ToDeviceVerificationCancel, m.HandleToDeviceEvent)

	// Policy list updating
	m.EventProcessor.On(event.StatePolicyUser, m.UpdatePolicyList)
	m.EventProcessor.On(event.StatePolicyRoom, m.UpdatePolicyList)
	m.EventProcessor.On(event.StatePolicyServer, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateLegacyPolicyUser, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateLegacyPolicyRoom, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateLegacyPolicyServer, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateUnstablePolicyUser, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateUnstablePolicyRoom, m.UpdatePolicyList)
	m.EventProcessor.On(event.StateUnstablePolicyServer, m.UpdatePolicyList)
	m.EventProcessor.On(event.EventRedaction, m.UpdatePolicyList)
	// Management room config
	m.EventProcessor.On(config.StateWatchedLists, m.HandleConfigChange)
	m.EventProcessor.On(config.StateProtectedRooms, m.HandleConfigChange)
	m.EventProcessor.On(event.StatePowerLevels, m.HandleConfigChange)
	// General event handling
	m.EventProcessor.On(event.StateMember, m.HandleMember)
	m.EventProcessor.On(event.EventMessage, m.HandleMessage)
	m.EventProcessor.On(event.EventSticker, m.HandleMessage)
	m.EventProcessor.On(event.EventEncrypted, m.HandleEncrypted)
}

func (m *Meowlnir) HandleToDeviceEvent(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleToDeviceEvent.evtx:", string(evtx))
	m.MapLock.RLock()
	bot, ok := m.Bots[evt.ToUserID]
	m.MapLock.RUnlock()
	if !ok {
		zerolog.Ctx(ctx).Warn().
			Stringer("user_id", evt.ToUserID).
			Stringer("device_id", evt.ToDeviceID).
			Msg("Received to-device event for unknown user")
	} else {
		bot.Mach.HandleToDeviceEvent(ctx, evt)
	}
}

func (m *Meowlnir) HandleOTKCounts(ctx context.Context, evt *mautrix.OTKCount) {
	m.MapLock.RLock()
	bot, ok := m.Bots[evt.UserID]
	m.MapLock.RUnlock()
	if !ok {
		zerolog.Ctx(ctx).Warn().
			Stringer("user_id", evt.UserID).
			Stringer("device_id", evt.DeviceID).
			Msg("Received OTK count for unknown user")
	} else {
		bot.Mach.HandleOTKCounts(ctx, evt)
	}
}

func (m *Meowlnir) UpdatePolicyList(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("UpdatePolicyList.evtx:", string(evtx))
	added, removed := m.PolicyStore.Update(evt)
	for _, eval := range m.EvaluatorByManagementRoom {
		eval.HandlePolicyListChange(ctx, evt.RoomID, added, removed)
	}
}

func (m *Meowlnir) HandleConfigChange(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleConfigChange.evtx:", string(evtx))
	m.MapLock.RLock()
	managementRoom, isManagement := m.EvaluatorByManagementRoom[evt.RoomID]
	protectedRoom, isProtected := m.EvaluatorByProtectedRoom[evt.RoomID]
	m.MapLock.RUnlock()
	if isManagement {
		managementRoom.HandleConfigChange(ctx, evt)
	} else if isProtected {
		protectedRoom.HandleProtectedRoomPowerLevels(ctx, evt)
	}
}

func (m *Meowlnir) HandleMember(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleMember.evtx:", string(evtx))
	content, ok := evt.Content.Parsed.(*event.MemberEventContent)
	if !ok {
		return
	}
	m.MapLock.RLock()
	bot, botOK := m.Bots[id.UserID(evt.GetStateKey())]
	m.MapLock.RUnlock()
	if botOK && content.Membership == event.MembershipInvite {
		_, err := bot.Client.JoinRoomByID(ctx, evt.RoomID)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Stringer("room_id", evt.RoomID).
				Stringer("inviter", evt.Sender).
				Msg("Failed to join management room after invite")
		} else {
			err = m.AddManagementRoom(ctx, bot.Meta.Username, evt.RoomID.String())
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Stringer("room_id", evt.RoomID).
					Stringer("inviter", evt.Sender).
					Msg("add management room")
			}
			zerolog.Ctx(ctx).Info().
				Stringer("room_id", evt.RoomID).
				Stringer("inviter", evt.Sender).
				Msg("Joined management room after invite, loading room state")
		}
	}
}

func (m *Meowlnir) HandleEncrypted(ctx context.Context, evt *event.Event) {

	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleEncrypted.evtx:", string(evtx))
	m.MapLock.RLock()
	botClient, isBot := m.Bots[""]
	//managementRoom, isManagement := m.EvaluatorByManagementRoom[evt.RoomID]
	//roomProtector, isProtected := m.EvaluatorByProtectedRoom[evt.RoomID]
	m.MapLock.RUnlock()
	if isBot {
		//		return
	}

	_ = botClient
	//else if isManagement {
	//fmt.Println("isManageMent:", isManagement)
	fmt.Println("to_user_id:", evt.ToUserID)
	fmt.Println("sender:", evt.Sender)
	if evt.ToUserID.String() != "" {
		fmt.Println("不为空")
		cryptohelper := CryptoHelperByBotUsername(ctx, m.AS, m.CryptoStoreDB, id.NewUserID("meowlnir00b_bot", "server.mtsocialdao.com"), m.Config.Meowlnir.PickleKey)
		HandleEncrypted(ctx, cryptohelper, evt)
		//cryptohelper.HandleEncrypted(ctx, evt)
		//	botClient.CryptoHelper.HandleEncrypted(ctx, evt)
		//_ = cryptohelper
	} else {
		fmt.Println("toUserId为空")
		cryptohelper := CryptoHelperByBotUsername(ctx, m.AS, m.CryptoStoreDB, id.NewUserID("meowlnir00b_bot", "server.mtsocialdao.com"), m.Config.Meowlnir.PickleKey)
		HandleEncrypted(ctx, cryptohelper, evt)
		//cryptohelper.HandleEncrypted(ctx, evt)
		//	botClient.CryptoHelper.HandleEncrypted(ctx, evt)
		//_ = cryptohelper

	}

	//tmpBot, ok := m.Bots["@meowlnir002_bot:server.mtsocialdao.com"]
	//if ok {
	//	fmt.Println("hit:", evt.ToUserID)
	//	tmpBot.CryptoHelper.HandleEncrypted(ctx, evt)
	//}
	//	}
	//
	//	} else if isProtected {
	//		fmt.Println("isProtected:", isProtected)
	//		roomProtector.HandleMessage(ctx, evt)
	//	}
}

//func HandleEncrypted(ctx context.Context, helper *cryptohelper.CryptoHelper, evt *event.Event) {
//	xx, _ := json.MarshalIndent(evt, " ", "\t")
//	fmt.Println("HandleEncrypted.evtx:", string(xx))
//	if helper == nil {
//		return
//	}
//	content := evt.Content.AsEncrypted()
//	helper.RequestSession(context.TODO(), evt.RoomID, content.SenderKey, content.SessionID, evt.Sender, content.DeviceID)
//	// TODO use context log instead of helper?
//	log := zerolog.Ctx(ctx).With().
//		Str("event_id", evt.ID.String()).
//		Str("session_id", content.SessionID.String()).
//		Logger()
//	log.Debug().Msg("Decrypting received event")
//	ctx = log.WithContext(ctx)
//
//	decrypted, err := helper.Decrypt(ctx, evt)
//	if err != nil {
//		log.Warn().Err(err).Msg("Failed to decrypt event")
//		helper.DecryptErrorCallback(evt, err)
//		return
//	}
//	evtx, _ := json.MarshalIndent(decrypted, " ", "\t")
//
//	fmt.Println("decrypted:", string(evtx))
//
//}

const initialSessionWaitTimeout = 3 * time.Second

func HandleEncrypted(ctx context.Context, helper *cryptohelper.CryptoHelper, evt *event.Event) {

	fmt.Println("decryptOlmEventdecryptOlmEventdecryptOlmEvent")

	//helper.RequestSession(context.TODO(), evt.RoomID, content.SenderKey, content.SessionID, evt.Sender, content.DeviceID)

	if helper == nil {
		return
	}
	evt.Content.ParseRaw(event.EventEncrypted)
	fmt.Println("evt.Content.AsEncrypted():", evt.Content.AsEncrypted())
	////if evt.Content.AsMessage().Body == ""
	//helper.RequestSession(ctx,
	//	evt.RoomID,
	//	evt.Content.AsEncrypted().SenderKey,
	//	evt.Content.AsEncrypted().SessionID, evt.Sender, evt.Content.AsMessage().FromDevice)
	//
	content := evt.Content.AsEncrypted()
	// TODO use context log instead of helper?
	log := zerolog.Ctx(ctx).With().
		Str("event_id", evt.ID.String()).
		Str("session_id", content.SessionID.String()).
		Logger()
	log.Debug().Msg("Decrypting received event")
	ctx = log.WithContext(ctx)

	//helper.Machine().HandleRoomKeyWithheld(ctx, &event.RoomKeyWithheldEventContent{
	//	RoomID:    evt.RoomID,
	//	Algorithm: content.Algorithm,
	//	SessionID: content.SessionID,
	//	SenderKey: content.SenderKey,
	//	Code:      event.RoomKeyWithheldUnverified,
	//	Reason:    "",
	//})
	//ddd, _ := json.MarshalIndent(evt, " ", "\t")
	//fmt.Println("ddd:", string(ddd))
	//func (mach *OlmMachine) HandleEncryptedEvent(ctx context.Context, evt *event.Event) {
	//	if _, ok := evt.Content.Parsed.(*event.EncryptedEventContent); !ok {

	decrypted, err := helper.Decrypt(ctx, evt)
	if errors.Is(err, cryptohelper.NoSessionFound) {
		log.Debug().
			Int("wait_seconds", int(initialSessionWaitTimeout.Seconds())).
			Msg("Couldn't find session, waiting for keys to arrive...")
		if helper.Machine().WaitForSession(ctx, evt.RoomID, content.SenderKey, content.SessionID, initialSessionWaitTimeout) {
			log.Debug().Msg("Got keys after waiting, trying to decrypt event again")
			decrypted, err = helper.Decrypt(ctx, evt)
		} else {
			fmt.Println("decrypted failed:")
			return
		}
	}
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decrypt event")
		helper.DecryptErrorCallback(evt, err)
		return
	}

	dd, _ := json.MarshalIndent(decrypted, " ", "\t")
	fmt.Println("dd:", string(dd))

}

func (m *Meowlnir) HandleMessage(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleMessage.evtx:", string(evtx))
	//content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	//if !ok {
	//	return
	//}
	//m.MapLock.RLock()
	//_, isBot := m.Bots[evt.Sender]
	//managementRoom, isManagement := m.EvaluatorByManagementRoom[evt.RoomID]
	//roomProtector, isProtected := m.EvaluatorByProtectedRoom[evt.RoomID]
	//m.MapLock.RUnlock()
	//if isBot {
	//	return
	//}
	////if isManagement {
	////	if content.MsgType == event.MsgText && managementRoom.Admins.Has(evt.Sender) {
	//managementRoom.HandleCommand(ctx, evt)
	////	}
	////} else if isProtected {
	//roomProtector.HandleMessage(ctx, evt)
	//}
}
