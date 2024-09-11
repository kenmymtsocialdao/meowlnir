package policyeval

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/meowlnir/config"
)

func (pe *PolicyEvaluator) GetProtectedRooms() []id.RoomID {
	pe.protectedRoomsLock.RLock()
	rooms := slices.Collect(maps.Keys(pe.protectedRooms))
	pe.protectedRoomsLock.RUnlock()
	return rooms
}

func (pe *PolicyEvaluator) IsProtectedRoom(roomID id.RoomID) bool {
	pe.protectedRoomsLock.RLock()
	_, protected := pe.protectedRooms[roomID]
	pe.protectedRoomsLock.RUnlock()
	return protected
}

func (pe *PolicyEvaluator) HandleProtectedRoomPowerLevels(ctx context.Context, evt *event.Event) {
	powerLevels := evt.Content.AsPowerLevels()
	ownLevel := powerLevels.GetUserLevel(pe.Bot.UserID)
	minLevel := max(powerLevels.Ban(), powerLevels.Redact())
	pe.protectedRoomsLock.RLock()
	_, isProtecting := pe.protectedRooms[evt.RoomID]
	_, wantToProtect := pe.wantToProtect[evt.RoomID]
	pe.protectedRoomsLock.RUnlock()
	if isProtecting && ownLevel < minLevel {
		pe.sendNotice(ctx, "⚠️ Bot no longer has sufficient power level in [%s](%s) (have %d, minimum %d)", evt.RoomID, evt.RoomID.URI().MatrixToURL(), ownLevel, minLevel)
	} else if wantToProtect && ownLevel >= minLevel {
		_, errMsg := pe.tryProtectingRoom(ctx, nil, evt.RoomID, true)
		if errMsg != "" {
			pe.sendNotice(ctx, "Retried protecting room after power level change, but failed: %s", strings.TrimPrefix(errMsg, "* "))
		} else {
			pe.sendNotice(ctx, "Power levels corrected, now protecting [%s](%s)", evt.RoomID, evt.RoomID.URI().MatrixToURL())
		}
	}
}

func (pe *PolicyEvaluator) tryProtectingRoom(ctx context.Context, joinedRooms *mautrix.RespJoinedRooms, roomID id.RoomID, doReeval bool) (*mautrix.RespMembers, string) {
	if claimer := pe.claimProtected(roomID, pe, true); claimer != pe {
		if claimer != nil && claimer.Bot.UserID == pe.Bot.UserID {
			return nil, fmt.Sprintf("* Room [%s](%s) is already protected by [%s](%s)", roomID, roomID.URI().MatrixToURL(), claimer.ManagementRoom, claimer.ManagementRoom.URI().MatrixToURL())
		} else {
			return nil, fmt.Sprintf("* Room [%s](%s) is already protected by another bot", roomID, roomID.URI().MatrixToURL())
		}
	}
	var err error
	if joinedRooms == nil {
		joinedRooms, err = pe.Bot.JoinedRooms(ctx)
		if err != nil {
			return nil, fmt.Sprintf("* Failed to get joined rooms: %v", err)
		}
	}
	pe.markAsWantToProtect(roomID)
	if !slices.Contains(joinedRooms.JoinedRooms, roomID) {
		return nil, fmt.Sprintf("* Bot is not in protected room [%s](%s)", roomID, roomID.URI().MatrixToURL())
	}
	var powerLevels event.PowerLevelsEventContent
	err = pe.Bot.StateEvent(ctx, roomID, event.StatePowerLevels, "", &powerLevels)
	if err != nil {
		return nil, fmt.Sprintf("* Failed to get power levels for [%s](%s): %v", roomID, roomID.URI().MatrixToURL(), err)
	}
	ownLevel := powerLevels.GetUserLevel(pe.Bot.UserID)
	minLevel := max(powerLevels.Ban(), powerLevels.Redact())
	if ownLevel < minLevel && !pe.DryRun {
		return nil, fmt.Sprintf("* Bot does not have sufficient power level in [%s](%s) (have %d, minimum %d)", roomID, roomID.URI().MatrixToURL(), ownLevel, minLevel)
	}
	members, err := pe.Bot.Members(ctx, roomID)
	if err != nil {
		return nil, fmt.Sprintf("* Failed to get room members for [%s](%s): %v", roomID, roomID.URI().MatrixToURL(), err)
	}
	pe.markAsProtectedRoom(roomID, members.Chunk)
	if doReeval {
		memberIDs := make([]id.UserID, len(members.Chunk))
		for i, member := range members.Chunk {
			memberIDs[i] = id.UserID(member.GetStateKey())
		}
		pe.EvaluateAllMembers(ctx, memberIDs)
	}
	return members, ""
}

func (pe *PolicyEvaluator) handleProtectedRooms(ctx context.Context, evt *event.Event, isInitial bool) (output, errors []string) {
	content, ok := evt.Content.Parsed.(*config.ProtectedRoomsEventContent)
	if !ok {
		return nil, []string{"* Failed to parse protected rooms event"}
	}
	pe.protectedRoomsLock.Lock()
	for roomID := range pe.protectedRooms {
		if !slices.Contains(content.Rooms, roomID) {
			delete(pe.protectedRooms, roomID)
			pe.claimProtected(roomID, pe, false)
			output = append(output, fmt.Sprintf("* Stopped protecting room [%s](%s)", roomID, roomID.URI().MatrixToURL()))
		}
	}
	pe.protectedRoomsLock.Unlock()
	joinedRooms, err := pe.Bot.JoinedRooms(ctx)
	if err != nil {
		return output, []string{"* Failed to get joined rooms: ", err.Error()}
	}
	var outLock sync.Mutex
	reevalMembers := make(map[id.UserID]struct{})
	var wg sync.WaitGroup
	for _, roomID := range content.Rooms {
		if pe.IsProtectedRoom(roomID) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			members, errMsg := pe.tryProtectingRoom(ctx, joinedRooms, roomID, false)
			outLock.Lock()
			defer outLock.Unlock()
			if errMsg != "" {
				errors = append(errors, errMsg)
			}
			if !isInitial {
				for _, member := range members.Chunk {
					reevalMembers[id.UserID(member.GetStateKey())] = struct{}{}
				}
				output = append(output, fmt.Sprintf("* Started protecting room [%s](%s)", roomID, roomID.URI().MatrixToURL()))
			}
		}()
	}
	wg.Wait()
	if len(reevalMembers) > 0 {
		pe.EvaluateAllMembers(ctx, slices.Collect(maps.Keys(reevalMembers)))
	}
	return
}

func (pe *PolicyEvaluator) markAsWantToProtect(roomID id.RoomID) {
	pe.protectedRoomsLock.Lock()
	defer pe.protectedRoomsLock.Unlock()
	pe.wantToProtect[roomID] = struct{}{}
}

func (pe *PolicyEvaluator) markAsProtectedRoom(roomID id.RoomID, evts []*event.Event) {
	pe.protectedRoomsLock.Lock()
	defer pe.protectedRoomsLock.Unlock()
	pe.protectedRooms[roomID] = struct{}{}
	delete(pe.wantToProtect, roomID)
	for _, evt := range evts {
		pe.unlockedUpdateUser(id.UserID(evt.GetStateKey()), evt.RoomID, evt.Content.AsMember().Membership)
	}
}

func isInRoom(membership event.Membership) bool {
	switch membership {
	case event.MembershipJoin, event.MembershipInvite, event.MembershipKnock:
		return true
	}
	return false
}

func (pe *PolicyEvaluator) updateUser(userID id.UserID, roomID id.RoomID, membership event.Membership) bool {
	pe.protectedRoomsLock.Lock()
	defer pe.protectedRoomsLock.Unlock()
	_, isProtected := pe.protectedRooms[roomID]
	if !isProtected {
		return false
	}
	return pe.unlockedUpdateUser(userID, roomID, membership)
}

func (pe *PolicyEvaluator) unlockedUpdateUser(userID id.UserID, roomID id.RoomID, membership event.Membership) bool {
	add := isInRoom(membership)
	if add {
		if !slices.Contains(pe.protectedRoomMembers[userID], roomID) {
			pe.protectedRoomMembers[userID] = append(pe.protectedRoomMembers[userID], roomID)
			return true
		}
	} else if idx := slices.Index(pe.protectedRoomMembers[userID], roomID); idx >= 0 {
		deleted := slices.Delete(pe.protectedRoomMembers[userID], idx, idx+1)
		if len(deleted) == 0 {
			delete(pe.protectedRoomMembers, userID)
		} else {
			pe.protectedRoomMembers[userID] = deleted
		}
	}
	return false
}
