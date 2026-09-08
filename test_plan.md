I will rewrite the `for _, z := range validZones` loop in `plugins/dns/sync.go` to use batch operations.

Original logic:
```go
	var created, updated int
	for _, z := range validZones {
		name := strings.ToLower(z.Name)
		if dom, exists := existingMap[name]; exists {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			p.db.Save(dom)
			updated++
			forgetOrigin(name)
		} else {
			if err := p.db.Create(&Domain{
				OrgID: p.orgID(r),
				Name:  name, ProviderAccountID: acc.ID, ZoneID: z.ID,
			}).Error; err == nil {
				created++
				forgetOrigin(name)
			}
		}
	}
```

New logic using batching:
```go
	var toUpdate []Domain
	var toCreate []Domain
	var created, updated int
	for _, z := range validZones {
		name := strings.ToLower(z.Name)
		if dom, exists := existingMap[name]; exists {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			toUpdate = append(toUpdate, *dom)
			updated++
			forgetOrigin(name)
		} else {
			toCreate = append(toCreate, Domain{
				OrgID:             p.orgID(r),
				Name:              name,
				ProviderAccountID: acc.ID,
				ZoneID:            z.ID,
			})
			forgetOrigin(name)
		}
	}

	if len(toUpdate) > 0 {
		p.db.Save(&toUpdate)
	}
	if len(toCreate) > 0 {
		res := p.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&toCreate)
		created = int(res.RowsAffected)
	}
```

Wait, `created` must reflect `RowsAffected`. If we do `clause.OnConflict{DoNothing: true}`, `RowsAffected` returns the number of actually inserted rows. This perfectly fixes the empirical test that checks that conflicts are ignored AND `created` is correctly incremented only for successful creations! Wait, I should make sure I import `"gorm.io/gorm/clause"` at the top.

Let's double check if I need to adjust `forgetOrigin(name)`. The original code calls `forgetOrigin(name)` only if `err == nil` on Create. If we batch Create, we call `forgetOrigin(name)` unconditionally before Create. If a domain wasn't created due to conflict, it will still call `forgetOrigin(name)` for it. `forgetOrigin` just does an in-memory cache eviction which is harmless if done unnecessarily.

Wait, `p.db.Save(&toUpdate)` will update all fields. Is that ok? The original code does `p.db.Save(dom)` which also updates all fields (since `dom` is exactly what came from the DB).

Wait, I should test my plan with a quick benchmark in the codebase and verify the `TestFinding4_SyncDomainsIgnoresDBCreateConflict` still passes.
