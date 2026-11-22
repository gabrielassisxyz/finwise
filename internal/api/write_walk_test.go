package api

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// changePin is the one write that is neither a generic handler's nor one of the
// registry's: it needs what only the session service has, the caller's token,
// the lockout and the session store.
const changePin = "session.changePin"

// writesWithoutHandler lists the writes and evaluations nothing implements yet.
// Each piece of work that implements one removes its name here, and the list is
// empty when every write of the contract is implemented.
var writesWithoutHandler = []string{
	"transactions.setLine", "transactions.applyLineToEstablishment",
	"transactions.setLineForMany",
	"transactions.confirmLine", "transactions.setReason",
	"transactions.undoHistoryStep", "subscriptions.create", "subscriptions.acceptSuggestion",
	"subscriptions.rejectSuggestion", "subscriptions.setStatus", "subscriptions.setLine",
	"subscriptions.setReviewDate", "subscriptions.setNote", "review.acceptPending", "review.approve",
	"review.refuse", "review.propose", "review.dismissWarning", "review.runWarningAction",
	"review.markStepDone", "review.keepSubscription", "review.cancelSubscription",
	"review.closeMonth", "review.markTransferDone", "settings.setNickname", "settings.setDefaultLine",
}

// sessionServiceWrites are the writes the session service answers itself. It
// holds changePin, and only that.
var sessionServiceWrites = []string{changePin}

// writesAndEvaluationsInTheManifest is the count the walk is pinned to: 55
// writes and one evaluation.
const writesAndEvaluationsInTheManifest = 56

func changingMethods(methods []contract.Method) []string {
	var names []string
	for _, method := range methods {
		if method.Class == contract.ClassWrite || method.Class == contract.ClassEvaluate {
			names = append(names, method.Qualified())
		}
	}
	return names
}

// walkProblems names every disagreement between the manifest's writes and
// evaluations and the three places that can implement one: the registry, the
// session service for changePin alone, and the list of those not yet
// implemented. A method is in exactly one of them.
func walkProblems(changing, registered, sessionService, listed []string) []string {
	inManifest := map[string]bool{}
	for _, name := range changing {
		inManifest[name] = true
	}
	places := map[string][]string{}
	var problems []string
	note := func(place string, names []string) {
		for _, name := range names {
			if !inManifest[name] {
				problems = append(problems, fmt.Sprintf("%s: in %s, which is not a write or an evaluation of the manifest", name, place))
				continue
			}
			places[name] = append(places[name], place)
		}
	}
	note("the registry", registered)
	note("the session service", sessionService)
	note("writesWithoutHandler", listed)
	for _, name := range changing {
		switch found := places[name]; len(found) {
		case 0:
			problems = append(problems, name+": neither implemented nor listed in writesWithoutHandler")
		case 1:
		default:
			problems = append(problems, name+": in "+strings.Join(found, " and ")+", and it belongs in one")
		}
	}
	if slices.Contains(registered, changePin) {
		problems = append(problems, changePin+": the registry holds a handler for it; the session service answers it")
	}
	for _, name := range sessionService {
		if name != changePin {
			problems = append(problems, name+": the session service answers session.changePin alone")
		}
	}
	sort.Strings(problems)
	return problems
}

func registeredChanges(t *testing.T, changing []string) []string {
	t.Helper()
	var registered []string
	for _, name := range NewReadRegistry().Names() {
		if slices.Contains(changing, name) {
			registered = append(registered, name)
		}
	}
	return registered
}

func manifestChanging(t *testing.T) []string {
	t.Helper()
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	return changingMethods(methods)
}

func TestEveryWriteAndEvaluationHasExactlyOneImplementation(t *testing.T) {
	changing := manifestChanging(t)
	if len(changing) != writesAndEvaluationsInTheManifest {
		t.Fatalf("the manifest has %d write and evaluate methods, want %d", len(changing), writesAndEvaluationsInTheManifest)
	}
	registered := registeredChanges(t, changing)
	for _, problem := range walkProblems(changing, registered, sessionServiceWrites, writesWithoutHandler) {
		t.Error(problem)
	}
	implemented := []string{
		"budget.applyScenario", "budget.evaluateScenario", "budget.save", "budget.saveScenario", "changes.undo",
		"settings.setCardLabel", "settings.setClosing", "settings.setMinExamples", "settings.setPotBucket",
		"settings.setThreshold", "settings.toggleLinkedLine",
		"transactions.addFixedTag", "transactions.addTag", "transactions.addTagToMany", "transactions.excludeMany",
		"transactions.removeFixedTag", "transactions.removeTag", "transactions.setDefaultLine", "transactions.setExcluded",
		"transactions.setExclusionReason", "transactions.setNickname", "transactions.setNote", "transactions.setRefund",
		"transactions.setRefundLine", "transactions.setSplit",
		"transactions.linkReimbursement", "transactions.setExpectedReimbursement", "transactions.setReimbursable",
		"transactions.unlinkReimbursement",
	}
	slices.Sort(implemented)
	if !slices.Equal(registered, implemented) {
		t.Errorf("the registry implements %v, want %v", registered, implemented)
	}
	if len(writesWithoutHandler) != 26 || slices.Contains(writesWithoutHandler, changePin) {
		t.Errorf("the list holds %d names, want 26 without %s", len(writesWithoutHandler), changePin)
	}
}

func TestTheWalkFailsWhenAMethodIsInNoPlaceOrInTwoOrChangePinHasAHandler(t *testing.T) {
	changing := []string{"a.one", "a.two", changePin}
	clean := walkProblems(changing, []string{"a.one"}, nil, []string{"a.two", changePin})
	if len(clean) != 0 {
		t.Fatalf("a consistent arrangement was reported: %v", clean)
	}
	cases := map[string]struct {
		registered, sessionService, listed []string
	}{
		"a name removed from the list":           {[]string{"a.one"}, nil, []string{changePin}},
		"a name in the registry and the list":    {[]string{"a.one", "a.two"}, nil, []string{"a.two", changePin}},
		"a name listed twice":                    {[]string{"a.one"}, nil, []string{"a.two", "a.two", changePin}},
		"a handler registered for changePin":     {[]string{"a.one", changePin}, nil, []string{"a.two"}},
		"changePin in the registry and the list": {[]string{"a.one", changePin}, nil, []string{"a.two", changePin}},
		"changePin in the session service too":   {[]string{"a.one"}, []string{changePin}, []string{"a.two", changePin}},
		"another name in the session service":    {[]string{"a.one"}, []string{"a.two"}, []string{changePin}},
		"a name that is not a write":             {[]string{"a.one"}, nil, []string{"a.two", changePin, "shell.overview"}},
	}
	for label, arrangement := range cases {
		if len(walkProblems(changing, arrangement.registered, arrangement.sessionService, arrangement.listed)) == 0 {
			t.Errorf("%s went unnoticed", label)
		}
	}
	if problems := walkProblems(changing, []string{"a.one"}, []string{changePin}, []string{"a.two"}); len(problems) != 0 {
		t.Errorf("changePin implemented by the session service was reported: %v", problems)
	}
}

// postBodies are four bodies that are not an argument list a method takes: the
// refusal must not depend on any of them.
var postBodies = []string{`[]`, `not json`, ``, `{"a":1}`}

// readsOfBody posts a method's route with a body that counts its reads.
func readsOfBody(env *testEnv, route string, cookie *http.Cookie) (reads, status int) {
	body := &countingBody{}
	request, _ := http.NewRequest(http.MethodPost, route, body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := newRecorder()
	env.handler.ServeHTTP(recorder, request)
	return body.reads, recorder.Code
}

func TestWithoutADatabaseEveryWriteAndEvaluationIsRefusedBeforeItsBodyIsRead(t *testing.T) {
	env := newTestEnv(t, func(config *Config) { config.Reads = NewReadRegistry() })
	cookie := env.unlock()
	refused := methodsOfClass(env, contract.ClassWrite, contract.ClassEvaluate)
	if len(refused) != writesAndEvaluationsInTheManifest {
		t.Fatalf("the manifest has %d write and evaluate methods, want %d", len(refused), writesAndEvaluationsInTheManifest)
	}
	for _, method := range refused {
		for _, body := range postBodies {
			expectError(t, method.Route+" body "+body, env.call(method.Route, body, cookie), http.StatusForbidden, "read_only")
		}
		if reads, status := readsOfBody(env, method.Route, cookie); reads != 0 || status != http.StatusForbidden {
			t.Errorf("%s: the body was read %d times and the status was %d, want 0 and 403", method.Route, reads, status)
		}
	}
}

func TestWithADatabaseAWriteWithNoHandlerIsRefusedBeforeItsBodyIsRead(t *testing.T) {
	scratch := authoredtest.Open(t, newFakeClock().Now)
	env := newTestEnv(t, withDatabase(scratch.Database), func(config *Config) { config.Reads = NewReadRegistry() })
	cookie := env.unlock()
	before := scratch.State(t)
	for _, name := range writesWithoutHandler {
		domain, method := splitMethod(t, name)
		route := "/api/" + domain + "/" + method
		for _, body := range postBodies {
			expectError(t, name+" body "+body, env.call(route, body, cookie), http.StatusForbidden, "read_only")
		}
		if reads, status := readsOfBody(env, route, cookie); reads != 0 || status != http.StatusForbidden {
			t.Errorf("%s: the body was read %d times and the status was %d, want 0 and 403", name, reads, status)
		}
	}
	if after := scratch.State(t); after != before {
		t.Errorf("a refused write changed the database:\n%s\nwas\n%s", after, before)
	}
}
