package policyeval

import (
	"context"
	"maps"
	"slices"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/meowlnir/database"
	"go.mau.fi/meowlnir/policylist"
)

func (pe *PolicyEvaluator) EvaluateAll(ctx context.Context) {
	pe.usersLock.RLock()
	users := slices.Collect(maps.Keys(pe.users))
	pe.usersLock.RUnlock()
	pe.EvaluateAllMembers(ctx, users)
}

func (pe *PolicyEvaluator) EvaluateAllMembers(ctx context.Context, members []id.UserID) {
	for _, member := range members {
		pe.EvaluateUser(ctx, member)
	}
}

func (pe *PolicyEvaluator) EvaluateUser(ctx context.Context, userID id.UserID) {
	match := pe.Store.MatchUser(pe.GetWatchedLists(), userID)
	if match == nil {
		return
	}
	zerolog.Ctx(ctx).Debug().
		Stringer("user_id", userID).
		Any("matches", match).
		Msg("Found matches for user")
	pe.ApplyPolicy(ctx, userID, match)
}

func (pe *PolicyEvaluator) EvaluateRemovedRule(ctx context.Context, policy *policylist.Policy) {
	if policy.Recommendation == event.PolicyRecommendationUnban {
		// When an unban rule is removed, evaluate all joined users against the removed rule
		// to see if they should be re-evaluated against all rules (and possibly banned)
		pe.usersLock.RLock()
		users := slices.Collect(maps.Keys(pe.users))
		pe.usersLock.RUnlock()
		for _, userID := range users {
			if policy.Pattern.Match(string(userID)) {
				pe.EvaluateUser(ctx, userID)
			}
		}
	} else {
		// For ban rules, find users who were banned by the rule and re-evaluate them.
		reevalTargets, err := pe.DB.TakenAction.GetAllByRuleEntity(ctx, policy.RoomID, policy.Entity)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Str("policy_entity", policy.Entity).
				Msg("Failed to get actions taken for removed policy")
			pe.sendNotice(ctx, "Database error in EvaluateRemovedRule (GetAllByRuleEntity): %v", err)
			return
		}
		pe.ReevaluateActions(ctx, reevalTargets)
	}
}

func (pe *PolicyEvaluator) EvaluateAddedRule(ctx context.Context, policy *policylist.Policy) {
	pe.usersLock.RLock()
	users := slices.Collect(maps.Keys(pe.users))
	pe.usersLock.RUnlock()
	for _, userID := range users {
		if policy.Pattern.Match(string(userID)) {
			pe.ApplyPolicy(ctx, userID, policylist.Match{policy})
		}
	}
}

func (pe *PolicyEvaluator) ReevaluateAffectedByLists(ctx context.Context, policyLists []id.RoomID) {
	var reevalTargets []*database.TakenAction
	for _, list := range policyLists {
		targets, err := pe.DB.TakenAction.GetAllByPolicyList(ctx, list)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Stringer("policy_list_id", list).
				Msg("Failed to get actions taken from policy list")
			pe.sendNotice(ctx, "Database error in ReevaluateAffectedByLists (GetAllByPolicyList): %v", err)
			continue
		}
		if reevalTargets == nil {
			reevalTargets = targets
		} else {
			reevalTargets = append(reevalTargets, targets...)
		}
	}
	pe.ReevaluateActions(ctx, reevalTargets)
}

func (pe *PolicyEvaluator) ReevaluateActions(ctx context.Context, actions []*database.TakenAction) {

}