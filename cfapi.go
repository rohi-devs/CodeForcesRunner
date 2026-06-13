package main

// ─────────────────────────────────────────────────────────────────────────────
//  cfapi.go  –  Codeforces REST API client
//
//  Flags (set by parseCLI in main.go, not flag.Parse):
//    --cf-user     <handle>    user info, rating history, recent submissions
//    --cf-contest  <id>        contest metadata + top-10 standings
//    --cf-problem  <2232F>     look up a specific problem by contestId+index
//    --cf-tags     <dp,greedy> search problemset by tags (comma-separated)
//    --cf-verdict  <id>        latest submission verdict in a contest
//    --cf-key / --cf-secret    API credentials (or CF_API_KEY / CF_API_SECRET)
//
//  Auth note:
//    - Public data (user info, ratings, problemset) works anonymously.
//    - contest.standings for regular (non-gym) contests MUST be fetched
//      anonymously with ONLY contestId — any extra param causes FAILED.
//      Use getAnon() for those calls.
//    - Authenticated calls (user.status, contest.status) use buildURL which
//      appends apiKey + time + SHA-512 apiSig per the CF API spec.
//
//  Rate limit: 1 req / 2 s. We sleep 500 ms before every call.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// ── CF API types ──────────────────────────────────────────────────────────────

type CFResponse struct {
	Status  string          `json:"status"`
	Comment string          `json:"comment"`
	Result  json.RawMessage `json:"result"`
}

type CFUser struct {
	Handle                  string `json:"handle"`
	FirstName               string `json:"firstName,omitempty"`
	LastName                string `json:"lastName,omitempty"`
	Country                 string `json:"country,omitempty"`
	Organization            string `json:"organization,omitempty"`
	Contribution            int    `json:"contribution"`
	Rank                    string `json:"rank"`
	Rating                  int    `json:"rating"`
	MaxRank                 string `json:"maxRank"`
	MaxRating               int    `json:"maxRating"`
	LastOnlineTimeSeconds   int64  `json:"lastOnlineTimeSeconds"`
	RegistrationTimeSeconds int64  `json:"registrationTimeSeconds"`
	FriendOfCount           int    `json:"friendOfCount"`
}

type CFProblem struct {
	ContestId int      `json:"contestId,omitempty"`
	Index     string   `json:"index"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Points    float64  `json:"points,omitempty"`
	Rating    int      `json:"rating,omitempty"`
	Tags      []string `json:"tags"`
}

type CFParty struct {
	ContestId       int        `json:"contestId,omitempty"`
	Members         []CFMember `json:"members"`
	ParticipantType string     `json:"participantType"`
	Ghost           bool       `json:"ghost"`
}

type CFMember struct {
	Handle string `json:"handle"`
	Name   string `json:"name,omitempty"`
}

type CFSubmission struct {
	Id                  int64     `json:"id"`
	ContestId           int       `json:"contestId,omitempty"`
	CreationTimeSeconds int64     `json:"creationTimeSeconds"`
	RelativeTimeSeconds int64     `json:"relativeTimeSeconds"`
	Problem             CFProblem `json:"problem"`
	Author              CFParty   `json:"author"`
	ProgrammingLanguage string    `json:"programmingLanguage"`
	Verdict             string    `json:"verdict,omitempty"`
	Testset             string    `json:"testset"`
	PassedTestCount     int       `json:"passedTestCount"`
	TimeConsumedMillis  int       `json:"timeConsumedMillis"`
	MemoryConsumedBytes int64     `json:"memoryConsumedBytes"`
	Points              float64   `json:"points,omitempty"`
}

type CFContest struct {
	Id                  int    `json:"id"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	Phase               string `json:"phase"`
	Frozen              bool   `json:"frozen"`
	DurationSeconds     int    `json:"durationSeconds"`
	StartTimeSeconds    int64  `json:"startTimeSeconds,omitempty"`
	RelativeTimeSeconds int64  `json:"relativeTimeSeconds,omitempty"`
	PreparedBy          string `json:"preparedBy,omitempty"`
	Difficulty          int    `json:"difficulty,omitempty"`
}

type CFRatingChange struct {
	ContestId               int    `json:"contestId"`
	ContestName             string `json:"contestName"`
	Handle                  string `json:"handle"`
	Rank                    int    `json:"rank"`
	RatingUpdateTimeSeconds int64  `json:"ratingUpdateTimeSeconds"`
	OldRating               int    `json:"oldRating"`
	NewRating               int    `json:"newRating"`
}

type CFProblemResult struct {
	Points                    float64 `json:"points"`
	Penalty                   int     `json:"penalty,omitempty"`
	RejectedAttemptCount      int     `json:"rejectedAttemptCount"`
	Type                      string  `json:"type"`
	BestSubmissionTimeSeconds int64   `json:"bestSubmissionTimeSeconds,omitempty"`
}

type CFRanklistRow struct {
	Party                 CFParty           `json:"party"`
	Rank                  int               `json:"rank"`
	Points                float64           `json:"points"`
	Penalty               int               `json:"penalty"`
	SuccessfulHackCount   int               `json:"successfulHackCount"`
	UnsuccessfulHackCount int               `json:"unsuccessfulHackCount"`
	ProblemResults        []CFProblemResult `json:"problemResults"`
}

type CFStandingsResult struct {
	Contest  CFContest       `json:"contest"`
	Problems []CFProblem     `json:"problems"`
	Rows     []CFRanklistRow `json:"rows"`
}

type CFProblemsResult struct {
	Problems          []CFProblem       `json:"problems"`
	ProblemStatistics []json.RawMessage `json:"problemStatistics"`
}

// ── API client ────────────────────────────────────────────────────────────────

type CFClient struct {
	apiKey    string
	apiSecret string
	baseURL   string
}

// newCFClient builds a client from the parsed CLI invocation + env fallback.
func newCFClient() *CFClient {
	return &CFClient{baseURL: "https://codeforces.com/api"}
}

// newCFClientWithCreds builds an authenticated client.
func newCFClientWithCreds(key, secret string) *CFClient {
	if key == "" {
		key = os.Getenv("CF_API_KEY")
	}
	if secret == "" {
		secret = os.Getenv("CF_API_SECRET")
	}
	return &CFClient{
		apiKey:    key,
		apiSecret: secret,
		baseURL:   "https://codeforces.com/api",
	}
}

func (c *CFClient) isAuthorized() bool {
	return c.apiKey != "" && c.apiSecret != ""
}

// buildURL builds a URL with optional API auth signature (CF spec SHA-512).
func (c *CFClient) buildURL(method string, params map[string]string) string {
	p := make(map[string]string, len(params))
	for k, v := range params {
		p[k] = v
	}

	if c.isAuthorized() {
		p["apiKey"] = c.apiKey
		p["time"] = fmt.Sprintf("%d", time.Now().Unix())
	}

	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(k))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(p[k]))
	}
	query := sb.String()

	if c.isAuthorized() {
		rb := make([]byte, 3)
		rand.Read(rb) //nolint:gosec
		rnd := fmt.Sprintf("%06x", rb)
		h := sha512.Sum512([]byte(fmt.Sprintf("%s/%s?%s#%s", rnd, method, query, c.apiSecret)))
		return fmt.Sprintf("%s/%s?%s&apiSig=%s", c.baseURL, method, query, url.QueryEscape(rnd+fmt.Sprintf("%x", h)))
	}
	return fmt.Sprintf("%s/%s?%s", c.baseURL, method, query)
}

// buildAnonURL builds a plain URL with no auth params at all.
// Required by CF for regular contest.standings — any extra param causes FAILED.
func (c *CFClient) buildAnonURL(method string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(k))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(params[k]))
	}
	return fmt.Sprintf("%s/%s?%s", c.baseURL, method, sb.String())
}

// get makes an authenticated (or unauthenticated if no creds) API call.
func (c *CFClient) get(method string, params map[string]string) (json.RawMessage, error) {
	return c.doGet(c.buildURL(method, params))
}

// getAnon makes a strictly anonymous API call — no auth params appended.
// Must be used for regular contest.standings (CF restriction).
func (c *CFClient) getAnon(method string, params map[string]string) (json.RawMessage, error) {
	return c.doGet(c.buildAnonURL(method, params))
}

func (c *CFClient) doGet(reqURL string) (json.RawMessage, error) {
	time.Sleep(500 * time.Millisecond) // CF rate limit: 1 req/2s
	logVerbose("CF GET %s", reqURL)

	resp, err := http.Get(reqURL) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var envelope CFResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if envelope.Status != "OK" {
		return nil, fmt.Errorf("CF API: %s", envelope.Comment)
	}
	return envelope.Result, nil
}

// ── Feature functions ─────────────────────────────────────────────────────────

func (c *CFClient) fetchUserInfo(handle string) error {
	fmt.Printf("\n┌─ Codeforces User: %s\n", handle)

	raw, err := c.get("user.info", map[string]string{"handles": handle})
	if err != nil {
		return err
	}
	var users []CFUser
	if err := json.Unmarshal(raw, &users); err != nil || len(users) == 0 {
		return fmt.Errorf("parse user.info: %w", err)
	}
	u := users[0]

	fmt.Printf("│  Handle       : %s\n", u.Handle)
	if u.FirstName != "" || u.LastName != "" {
		fmt.Printf("│  Name         : %s %s\n", u.FirstName, u.LastName)
	}
	if u.Country != "" {
		fmt.Printf("│  Country      : %s\n", u.Country)
	}
	if u.Organization != "" {
		fmt.Printf("│  Organization : %s\n", u.Organization)
	}
	fmt.Printf("│  Rank         : %s  (max: %s)\n", u.Rank, u.MaxRank)
	fmt.Printf("│  Rating       : %d  (max: %d)\n", u.Rating, u.MaxRating)
	fmt.Printf("│  Contribution : %+d\n", u.Contribution)
	fmt.Printf("│  Friends of   : %d users\n", u.FriendOfCount)
	fmt.Printf("│  Registered   : %s\n", time.Unix(u.RegistrationTimeSeconds, 0).Format("2006-01-02"))
	fmt.Printf("│  Last online  : %s\n", time.Unix(u.LastOnlineTimeSeconds, 0).UTC().Format("2006-01-02 15:04 UTC"))

	// Rating history — last 3
	if raw2, err := c.get("user.rating", map[string]string{"handle": handle}); err == nil {
		var changes []CFRatingChange
		if json.Unmarshal(raw2, &changes) == nil && len(changes) > 0 {
			fmt.Printf("│\n│  Recent rating changes (last 3):\n")
			start := len(changes) - 3
			if start < 0 {
				start = 0
			}
			for _, rc := range changes[start:] {
				delta := rc.NewRating - rc.OldRating
				sign := "+"
				if delta < 0 {
					sign = ""
				}
				ts := time.Unix(rc.RatingUpdateTimeSeconds, 0).Format("2006-01-02")
				fmt.Printf("│    [%s] %-45s rank #%-5d  %d → %d (%s%d)\n",
					ts, truncate(rc.ContestName, 45), rc.Rank,
					rc.OldRating, rc.NewRating, sign, delta)
			}
		}
	}

	// Recent submissions — last 5
	if raw3, err := c.get("user.status", map[string]string{
		"handle": handle, "from": "1", "count": "5",
	}); err == nil {
		var subs []CFSubmission
		if json.Unmarshal(raw3, &subs) == nil && len(subs) > 0 {
			fmt.Printf("│\n│  Recent submissions (last %d):\n", len(subs))
			for _, s := range subs {
				ts := time.Unix(s.CreationTimeSeconds, 0).Format("2006-01-02 15:04")
				verdict := s.Verdict
				if verdict == "" {
					verdict = "TESTING"
				}
				ref := fmt.Sprintf("%d%s", s.Problem.ContestId, s.Problem.Index)
				if s.Problem.ContestId == 0 {
					ref = s.Problem.Index
				}
				fmt.Printf("│    [%s] %-7s %-28s %-26s %s\n",
					ts, ref,
					truncate(s.Problem.Name, 28),
					truncate(s.ProgrammingLanguage, 26),
					verdictColor(verdict))
			}
		}
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

func (c *CFClient) fetchContest(contestId int) error {
	fmt.Printf("\n┌─ Codeforces Contest #%d\n", contestId)

	// Must be anonymous for regular contests — CF rejects any extra params.
	raw, err := c.getAnon("contest.standings", map[string]string{
		"contestId": fmt.Sprintf("%d", contestId),
	})
	if err != nil {
		return err
	}

	var result CFStandingsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse standings: %w", err)
	}

	ct := result.Contest
	startStr := "TBA"
	if ct.StartTimeSeconds > 0 {
		startStr = time.Unix(ct.StartTimeSeconds, 0).UTC().Format("2006-01-02 15:04 UTC")
	}
	dur := time.Duration(ct.DurationSeconds) * time.Second

	fmt.Printf("│  Name      : %s\n", ct.Name)
	fmt.Printf("│  Type      : %s    Phase: %s\n", ct.Type, ct.Phase)
	fmt.Printf("│  Start     : %s\n", startStr)
	fmt.Printf("│  Duration  : %s\n", fmtDuration(dur))
	if ct.PreparedBy != "" {
		fmt.Printf("│  Author    : %s\n", ct.PreparedBy)
	}
	if ct.Difficulty > 0 {
		fmt.Printf("│  Difficulty: %d/5\n", ct.Difficulty)
	}

	if len(result.Problems) > 0 {
		fmt.Printf("│\n│  Problems:\n")
		for _, p := range result.Problems {
			rating := ""
			if p.Rating > 0 {
				rating = fmt.Sprintf("  ★%d", p.Rating)
			}
			fmt.Printf("│    %s. %s%s\n", p.Index, p.Name, rating)
		}
	}

	// Top 10 rows
	rows := result.Rows
	if len(rows) > 10 {
		rows = rows[:10]
	}
	if len(rows) > 0 {
		fmt.Printf("│\n│  Standings (top %d of %d):\n", len(rows), len(result.Rows))
		fmt.Printf("│  %-5s %-20s %-8s %-8s\n", "Rank", "Handle", "Points", "Penalty")
		fmt.Printf("│  %-5s %-20s %-8s %-8s\n", "────", "──────────────────", "──────", "───────")
		for _, row := range rows {
			handle := "?"
			if len(row.Party.Members) > 0 {
				handle = row.Party.Members[0].Handle
			}
			fmt.Printf("│  %-5d %-20s %-8.0f %-8d\n",
				row.Rank, truncate(handle, 20), row.Points, row.Penalty)
		}
	}

	// Rating change summary (best-effort — available after contest ends)
	if raw2, err := c.getAnon("contest.ratingChanges", map[string]string{
		"contestId": fmt.Sprintf("%d", contestId),
	}); err == nil {
		var changes []CFRatingChange
		if json.Unmarshal(raw2, &changes) == nil && len(changes) > 0 {
			up, down, same := 0, 0, 0
			for _, rc := range changes {
				switch d := rc.NewRating - rc.OldRating; {
				case d > 0:
					up++
				case d < 0:
					down++
				default:
					same++
				}
			}
			fmt.Printf("│\n│  Rating changes: ↑%d  ↓%d  =%d  (total %d rated)\n",
				up, down, same, len(changes))
		}
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// fetchProblem looks up a single problem by ID, e.g. "2232F".
func (c *CFClient) fetchProblem(problemID string) error {
	problemID = strings.ToUpper(strings.TrimSpace(problemID))
	fmt.Printf("\n┌─ Codeforces Problem: %s\n", problemID)

	// Split leading digits (contestId) from trailing letter(s) (index).
	i := 0
	for i < len(problemID) && problemID[i] >= '0' && problemID[i] <= '9' {
		i++
	}
	if i == 0 || i == len(problemID) {
		return fmt.Errorf("invalid problem ID %q — expected digits + letter, e.g. 2232F", problemID)
	}
	contestIDStr := problemID[:i]
	index := problemID[i:]

	raw, err := c.getAnon("contest.standings", map[string]string{
		"contestId": contestIDStr,
	})
	if err != nil {
		return fmt.Errorf("fetch contest %s: %w", contestIDStr, err)
	}

	var result CFStandingsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse standings: %w", err)
	}

	var found *CFProblem
	for j := range result.Problems {
		if strings.EqualFold(result.Problems[j].Index, index) {
			found = &result.Problems[j]
			break
		}
	}
	if found == nil {
		available := make([]string, len(result.Problems))
		for j, p := range result.Problems {
			available[j] = p.Index
		}
		return fmt.Errorf("problem %s not found in contest %s (available: %s)",
			index, contestIDStr, strings.Join(available, ", "))
	}

	fmt.Printf("│  Contest : #%d — %s\n", result.Contest.Id, result.Contest.Name)
	fmt.Printf("│  Problem : %s — %s\n", found.Index, found.Name)
	if found.Rating > 0 {
		fmt.Printf("│  Rating  : %d\n", found.Rating)
	} else {
		fmt.Printf("│  Rating  : (unrated)\n")
	}
	if found.Points > 0 {
		fmt.Printf("│  Points  : %.0f\n", found.Points)
	}
	fmt.Printf("│  Type    : %s\n", found.Type)
	if len(found.Tags) > 0 {
		fmt.Printf("│  Tags    : %s\n", strings.Join(found.Tags, ", "))
	}
	fmt.Printf("│  URL     : https://codeforces.com/contest/%s/problem/%s\n", contestIDStr, index)

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// fetchTags searches the problemset by tags (comma-separated).
func (c *CFClient) fetchTags(tags string) error {
	parts := strings.Split(tags, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	cfTags := strings.Join(parts, ";") // CF uses semicolons

	fmt.Printf("\n┌─ Codeforces Problemset  [tags: %s]\n", tags)

	raw, err := c.get("problemset.problems", map[string]string{"tags": cfTags})
	if err != nil {
		return err
	}

	var result CFProblemsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse problems: %w", err)
	}

	total := len(result.Problems)
	if total == 0 {
		fmt.Printf("│  No problems found for tags: %s\n", tags)
		fmt.Println("└────────────────────────────────────────────────────────────────────────────")
		return nil
	}

	// Show 15 most recent (CF returns oldest-first)
	limit := 15
	if total < limit {
		limit = total
	}
	fmt.Printf("│  Found %d problems. Showing %d most recent:\n", total, limit)
	fmt.Printf("│  %-10s %-35s %-6s %s\n", "ID", "Name", "Rating", "Tags")
	fmt.Printf("│  %-10s %-35s %-6s %s\n",
		"──────────", "───────────────────────────────────", "──────", "────────────────────────────────────────")

	shown := result.Problems[total-limit:]
	for i := len(shown) - 1; i >= 0; i-- {
		p := shown[i]
		id := fmt.Sprintf("%d%s", p.ContestId, p.Index)
		if p.ContestId == 0 {
			id = p.Index
		}
		ratingStr := "-"
		if p.Rating > 0 {
			ratingStr = fmt.Sprintf("%d", p.Rating)
		}
		fmt.Printf("│  %-10s %-35s %-6s %s\n",
			id, truncate(p.Name, 35), ratingStr, truncate(strings.Join(p.Tags, ", "), 40))
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// fetchVerdict shows the single latest submission in a contest.
func (c *CFClient) fetchVerdict(contestId int) error {
	fmt.Printf("\n┌─ Latest Submission  [contest #%d]\n", contestId)

	raw, err := c.get("contest.status", map[string]string{
		"contestId": fmt.Sprintf("%d", contestId),
		"from":      "1",
		"count":     "1",
	})
	if err != nil {
		return err
	}

	var subs []CFSubmission
	if err := json.Unmarshal(raw, &subs); err != nil || len(subs) == 0 {
		fmt.Println("│  No submissions found.")
		fmt.Println("└────────────────────────────────────────────────────────────────────────────")
		return nil
	}

	s := subs[0]
	verdict := s.Verdict
	if verdict == "" {
		verdict = "TESTING"
	}
	handle := "?"
	if len(s.Author.Members) > 0 {
		handle = s.Author.Members[0].Handle
	}

	fmt.Printf("│  ID       : %d\n", s.Id)
	fmt.Printf("│  Time     : %s\n", time.Unix(s.CreationTimeSeconds, 0).UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("│  Author   : %s\n", handle)
	fmt.Printf("│  Problem  : %d%s — %s\n", s.Problem.ContestId, s.Problem.Index, s.Problem.Name)
	fmt.Printf("│  Language : %s\n", s.ProgrammingLanguage)
	fmt.Printf("│  Verdict  : %s\n", verdictColor(verdict))
	if verdict == "OK" || verdict == "PARTIAL" {
		fmt.Printf("│  Tests    : %d passed\n", s.PassedTestCount)
	}
	fmt.Printf("│  Time     : %d ms\n", s.TimeConsumedMillis)
	fmt.Printf("│  Memory   : %d KB\n", s.MemoryConsumedBytes/1024)

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// ── Verdict colour ────────────────────────────────────────────────────────────

func verdictColor(verdict string) string {
	const (
		green  = "\033[32m"
		red    = "\033[31m"
		yellow = "\033[33m"
		reset  = "\033[0m"
	)
	switch verdict {
	case "OK":
		return green + "Accepted" + reset
	case "WRONG_ANSWER":
		return red + "Wrong Answer" + reset
	case "TIME_LIMIT_EXCEEDED":
		return red + "Time Limit Exceeded" + reset
	case "MEMORY_LIMIT_EXCEEDED":
		return red + "Memory Limit Exceeded" + reset
	case "RUNTIME_ERROR":
		return red + "Runtime Error" + reset
	case "COMPILATION_ERROR":
		return red + "Compilation Error" + reset
	case "IDLENESS_LIMIT_EXCEEDED":
		return red + "Idleness Limit Exceeded" + reset
	case "PARTIAL":
		return yellow + "Partial" + reset
	case "TESTING", "SUBMITTED":
		return yellow + "Testing…" + reset
	case "CHALLENGED":
		return red + "Challenged" + reset
	case "SKIPPED":
		return yellow + "Skipped" + reset
	default:
		return verdict
	}
}

// ── runCFCommands — dispatch CF API flags from main ───────────────────────────

func runCFCommands(inv cliInvocation) bool {
	c := newCFClientWithCreds(inv.cfKey, inv.cfSecret)
	ran := false

	if inv.cfUser != "" {
		ran = true
		if err := c.fetchUserInfo(inv.cfUser); err != nil {
			fmt.Fprintf(os.Stderr, "cf-user: %v\n", err)
		}
	}
	if inv.cfContest != 0 {
		ran = true
		if err := c.fetchContest(inv.cfContest); err != nil {
			fmt.Fprintf(os.Stderr, "cf-contest: %v\n", err)
		}
	}
	if inv.cfProblem != "" {
		ran = true
		if err := c.fetchProblem(inv.cfProblem); err != nil {
			fmt.Fprintf(os.Stderr, "cf-problem: %v\n", err)
		}
	}
	if inv.cfTags != "" {
		ran = true
		if err := c.fetchTags(inv.cfTags); err != nil {
			fmt.Fprintf(os.Stderr, "cf-tags: %v\n", err)
		}
	}
	if inv.cfVerdict != 0 {
		ran = true
		if err := c.fetchVerdict(inv.cfVerdict); err != nil {
			fmt.Fprintf(os.Stderr, "cf-verdict: %v\n", err)
		}
	}
	return ran
}

// ── Shared helpers ────────────────────────────────────────────────────────────

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}