package config

import (
	"reflect"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	StateWatchedLists   = event.Type{Type: "fi.mau.meowlnir.watched_lists", Class: event.StateEventType}
	StateProtectedRooms = event.Type{Type: "fi.mau.meowlnir.protected_rooms", Class: event.StateEventType}
)

type WatchedPolicyList struct {
	Name string `json:"name"`
}

type WatchedListsEventContent struct {
	Lists map[id.RoomID]WatchedPolicyList `json:"lists"`
}

type ProtectedRoomsEventContent struct {
	Rooms []id.RoomID `json:"rooms"`
}

func init() {
	event.TypeMap[StateWatchedLists] = reflect.TypeOf(WatchedListsEventContent{})
	event.TypeMap[StateProtectedRooms] = reflect.TypeOf(ProtectedRoomsEventContent{})
}