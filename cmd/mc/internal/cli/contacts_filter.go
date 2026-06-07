package cli

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

const defaultContactSort = "name"

type contactListQuery struct {
	typeFilter meshcore.ContactType
	hasType    bool
	withinKM   float64
	hasWithin  bool
	sortBy     string
}

func (q contactListQuery) filtered() bool {
	return q.hasType || q.hasWithin
}

func contactListQueryFromEnv(e *env) (contactListQuery, error) {
	q := contactListQuery{sortBy: defaultContactSort}
	if t := e.args.flag("type"); t != "" {
		ct, err := parseContactTypeFilter(t)
		if err != nil {
			return q, err
		}
		q.typeFilter = ct
		q.hasType = true
	}
	if w := e.args.flag("within"); w != "" {
		km, err := parseWithinKM(w)
		if err != nil {
			return q, err
		}
		q.withinKM = km
		q.hasWithin = true
	}
	if s := e.args.flag("sort"); s != "" {
		sortBy, err := parseContactSort(s)
		if err != nil {
			return q, err
		}
		q.sortBy = sortBy
	}
	return q, nil
}

func parseContactSort(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "name", "type", "age", "adv", "route", "key", "distance":
		return strings.ToLower(strings.TrimSpace(s)), nil
	default:
		return "", fmt.Errorf("unknown --sort %q (supported: name, type, age, adv, route, key, distance)", s)
	}
}

func parseContactTypeFilter(s string) (meshcore.ContactType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "companion", "chat", "client":
		return meshcore.ContactChat, nil
	case "repeater":
		return meshcore.ContactRepeater, nil
	case "room":
		return meshcore.ContactRoom, nil
	case "sensor":
		return meshcore.ContactSensor, nil
	default:
		return "", fmt.Errorf("unknown --type %q (use companion, repeater, room, or sensor)", s)
	}
}

func parseWithinKM(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty --within value")
	}
	switch {
	case strings.HasSuffix(s, "km"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "km")), 64)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("invalid --within %q (use e.g. 10km)", s)
		}
		return v, nil
	case strings.HasSuffix(s, "m"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "m")), 64)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("invalid --within %q (use e.g. 500m)", s)
		}
		return v / 1000, nil
	default:
		return 0, fmt.Errorf("invalid --within %q (use e.g. 10km or 500m)", s)
	}
}

func filterContacts(contacts []meshcore.Contact, q contactListQuery, originLat, originLon float64) ([]meshcore.Contact, error) {
	out := append([]meshcore.Contact(nil), contacts...)
	if q.hasWithin && !ui.HasCoordinates(originLat, originLon) {
		return nil, fmt.Errorf("local companion coordinates are unavailable")
	}
	if q.sortBy == "distance" && !ui.HasCoordinates(originLat, originLon) {
		return nil, fmt.Errorf("local companion coordinates are unavailable")
	}

	if q.hasType {
		filtered := make([]meshcore.Contact, 0, len(out))
		for _, ct := range out {
			if ct.Type == q.typeFilter {
				filtered = append(filtered, ct)
			}
		}
		out = filtered
	}
	if q.hasWithin {
		filtered := make([]meshcore.Contact, 0, len(out))
		for _, ct := range out {
			if !ui.HasCoordinates(ct.Latitude, ct.Longitude) {
				continue
			}
			if contactDistanceKM(ct, originLat, originLon) <= q.withinKM {
				filtered = append(filtered, ct)
			}
		}
		out = filtered
	}
	sortContacts(out, q.sortBy, originLat, originLon)
	return out, nil
}

func sortContacts(contacts []meshcore.Contact, sortBy string, originLat, originLon float64) {
	sort.SliceStable(contacts, func(i, j int) bool {
		a, b := contacts[i], contacts[j]
		switch sortBy {
		case "type":
			ta, tb := ui.HumanContactType(a.Type), ui.HumanContactType(b.Type)
			if ta != tb {
				return ta < tb
			}
		case "age":
			na, nb := contactAgeNever(a.LastAdvert), contactAgeNever(b.LastAdvert)
			if na && nb {
				return contactNameLess(a, b)
			}
			if na {
				return false
			}
			if nb {
				return true
			}
			if !a.LastAdvert.Equal(b.LastAdvert) {
				return a.LastAdvert.Before(b.LastAdvert)
			}
		case "adv":
			aa, ab := contactAdvRank(a), contactAdvRank(b)
			if aa != ab {
				return aa < ab
			}
		case "route":
			ra, rb := contactRouteRank(a), contactRouteRank(b)
			if ra != rb {
				return ra < rb
			}
		case "key":
			ka, kb := strings.ToLower(a.PublicKey), strings.ToLower(b.PublicKey)
			if ka == "" && kb == "" {
				return contactNameLess(a, b)
			}
			if ka == "" {
				return false
			}
			if kb == "" {
				return true
			}
			if ka != kb {
				return ka < kb
			}
		case "distance":
			da := contactDistanceKM(a, originLat, originLon)
			db := contactDistanceKM(b, originLat, originLon)
			if math.IsNaN(da) && math.IsNaN(db) {
				return contactNameLess(a, b)
			}
			if math.IsNaN(da) {
				return false
			}
			if math.IsNaN(db) {
				return true
			}
			if da != db {
				return da < db
			}
		default:
			// name is the default sort key
		}
		return contactNameLess(a, b)
	})
}

func contactNameLess(a, b meshcore.Contact) bool {
	an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
	if an != bn {
		return an < bn
	}
	return strings.ToLower(a.PublicKey) < strings.ToLower(b.PublicKey)
}

func contactAgeNever(t time.Time) bool {
	return t.IsZero()
}

func contactAdvRank(ct meshcore.Contact) int {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return 0
	}
	return pathhash.HashSizeFromPathMeta(ct.OutPathEnc)
}

func contactRouteRank(ct meshcore.Contact) int {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return 1000
	}
	return pathhash.HopCountFromPathMeta(ct.OutPathEnc)
}

func contactDistanceKM(ct meshcore.Contact, originLat, originLon float64) float64 {
	if !ui.HasCoordinates(originLat, originLon) || !ui.HasCoordinates(ct.Latitude, ct.Longitude) {
		return math.NaN()
	}
	return ui.DistanceKM(originLat, originLon, ct.Latitude, ct.Longitude)
}
