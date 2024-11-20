package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
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
	_, isBot := m.Bots[evt.Sender]
	managementRoom, isManagement := m.EvaluatorByManagementRoom[evt.RoomID]
	//roomProtector, isProtected := m.EvaluatorByProtectedRoom[evt.RoomID]
	m.MapLock.RUnlock()
	if isBot {
		return
	}

	if isManagement {
		managementRoom.Bot.CryptoHelper.HandleEncrypted(ctx, evt)
	}

}

func (m *Meowlnir) HandleMessage(ctx context.Context, evt *event.Event) {
	evtx, _ := json.MarshalIndent(evt, " ", "\t")
	fmt.Println("HandleMessage.evtx:", string(evtx))
	evtBody, _ := json.Marshal(evt)
	req, _ := http.NewRequest("POST", m.Config.WebhookConfig.Url, bytes.NewReader(evtBody))
	resp, err := m.HttpClient.Do(req)
	if err != nil {
		m.Log.Err(err).Any("url", m.Config.WebhookConfig.Url).Msg("send event to webhook bridge failed")
		return
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)
	m.Log.Info().Any("resp", string(respData)).Msg("send event to webhook bridge successfully")
}
