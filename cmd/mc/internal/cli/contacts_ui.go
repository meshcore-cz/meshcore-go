package cli

import (
	"context"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func contactRowsFromMeshcore(contacts []meshcore.Contact, originLat, originLon float64, wide bool) []ui.ContactRow {
	rows := make([]ui.ContactRow, len(contacts))
	for i, ct := range contacts {
		rows[i] = ui.ContactRowFromMeshcore(ct, originLat, originLon, wide)
	}
	return rows
}

func contactListMetaFromBackend(backend Backend, count int) (ui.ContactListMeta, string) {
	meta := ui.ContactListMeta{Count: count}
	ib, ok := backend.(*ipcBackend)
	if !ok {
		return meta, ""
	}
	cs := ib.status.Contacts
	meta.Syncing = cs.Syncing
	meta.SyncReceived = cs.SyncReceived
	meta.SyncTotal = cs.SyncTotal
	meta.SyncedAt = cs.SyncedAt
	meta.Error = cs.Error
	if cs.Error != "" {
		meta.Cached = true
	}
	if !ib.status.Healthy {
		meta.Cached = true
		return meta, "showing cached contacts; backend is offline."
	}
	return meta, ""
}

func localOriginFromBackend(ctx context.Context, backend Backend) (lat, lon float64) {
	if ib, ok := backend.(*ipcBackend); ok {
		if ui.HasCoordinates(ib.status.Device.Latitude, ib.status.Device.Longitude) {
			return ib.status.Device.Latitude, ib.status.Device.Longitude
		}
	}
	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return 0, 0
	}
	return info.Latitude, info.Longitude
}

func printContactsHuman(ctx context.Context, e *env, backend Backend, contacts []meshcore.Contact, wide bool) {
	meta, warning := contactListMetaFromBackend(backend, len(contacts))
	if warning != "" {
		e.out.Human("Warning: %s\n", warning)
	}
	originLat, originLon := localOriginFromBackend(ctx, backend)
	data := ui.ContactListData{
		Contacts: contactRowsFromMeshcore(contacts, originLat, originLon, wide),
		Meta:     meta,
		Wide:     wide,
	}
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderContacts(data, printer))
}

func printContactDetailHuman(ctx context.Context, e *env, backend Backend, ct meshcore.Contact) {
	originLat, originLon := localOriginFromBackend(ctx, backend)
	data := ui.ContactDetailFromMeshcore(ct, originLat, originLon)
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderContactDetail(data, printer))
}
