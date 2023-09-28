package rrd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cobra"
)

type Config struct {
	ConfigCollection string
	RingName         string
	SizeColumnName   string
}

func MustRegister(app core.App, rootCmd *cobra.Command, cfg Config) {
	if err := Register(app, rootCmd, cfg); err != nil {
		panic(err)
	}
}

func Register(app core.App, rootCmd *cobra.Command, cfg Config) error {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		return manageRRDs(e.App, cfg)
	})
	return nil
}

type rrd struct {
	CollectionName string
	Size           int  `json:"size"`
	OldestRowId    *int `db:"oldest_rowid" json:"oldest"`
	NewestRowId    *int `db:"newest_rowid" json:"newest"`
	RecordCount    *int `db:"record_count" json:"count"`
}

func enforceRRDOnCollection(app core.App, cfg Config, record *models.Record) (collectionName string, unsub func()) {
	var rrd = rrd{
		Size:        record.GetInt(cfg.SizeColumnName),
		OldestRowId: types.Pointer(0),
		NewestRowId: types.Pointer(0),
		RecordCount: types.Pointer(0),
	}

	if coll, err := app.Dao().FindCollectionByNameOrId(record.GetString(cfg.RingName)); err == nil {
		rrd.CollectionName = coll.Name
		collectionName = coll.Name
	} else {
		fmt.Printf("Cannot enforce %s RRD %s: collection does not exist\n", cfg.ConfigCollection, record.GetString(cfg.RingName))
		return
	}
	if err := app.Dao().ConcurrentDB().
		NewQuery("SELECT MIN(rowid) AS oldest_rowid, MAX(rowid) AS newest_rowid, COUNT(*) AS record_count FROM " + rrd.CollectionName).
		One(&rrd); err != nil {
		panic(err)
	}
	if *rrd.RecordCount > rrd.Size {
		if _, err := app.Dao().ConcurrentDB().
			NewQuery(fmt.Sprintf("DELETE FROM %s WHERE rowid <= %d",
				rrd.CollectionName,
				*rrd.NewestRowId-rrd.Size,
			)).
			Execute(); err != nil {
			panic(err)
		}
	}
	json.NewEncoder(os.Stdout).Encode(rrd)

	onCreateHookId := app.OnModelBeforeCreate(rrd.CollectionName).
		Add(func(e *core.ModelEvent) error {
			if *rrd.RecordCount == 0 {
				rrd.RecordCount = types.Pointer(1)
				rrd.NewestRowId = types.Pointer(1)
				rrd.OldestRowId = types.Pointer(1)
				return nil

			} else if *rrd.RecordCount < rrd.Size {
				*rrd.RecordCount++
				*rrd.OldestRowId++
				return nil
				//This isn't the oldest row id
			} else if vals, ok := e.Model.(models.ColumnValueMapper); !ok {
			} else {
				params := vals.ColumnValueMap()
				//Convert a create into an update
				delete(params, "id")

				if _, err := app.Dao().
					NonconcurrentDB().
					Update(
						rrd.CollectionName,
						params,
						dbx.HashExp{
							"rowid": *rrd.OldestRowId % rrd.Size,
						}).Execute(); err != nil {
					return err
				} else {
					*rrd.OldestRowId++
				}
			}

			return hook.StopPropagation
		})
	onDeleteHookId := app.OnModelBeforeDelete(rrd.CollectionName).Add(func(e *core.ModelEvent) error {
		err := fmt.Errorf("rrd collection is insert and read only")
		fmt.Println(err)
		return err
	})
	onUpdateHookId := app.OnModelBeforeUpdate(rrd.CollectionName).Add(func(e *core.ModelEvent) error {
		err := fmt.Errorf("rrd collection is insert and read only")
		fmt.Println(err)
		return err
	})

	unsub = func() {
		app.OnRecordBeforeCreateRequest().Remove(onCreateHookId)
		app.OnRecordBeforeDeleteRequest().Remove(onDeleteHookId)
		app.OnRecordBeforeUpdateRequest().Remove(onUpdateHookId)
	}
	return
}

func manageRRDs(app core.App, cfg Config) error {

	unsubByCollection := map[string]func(){}
	if rrdCfgColl, err := app.Dao().
		FindCollectionByNameOrId(cfg.ConfigCollection); err == nil {

		app.OnRecordAfterDeleteRequest(cfg.ConfigCollection).Add(func(e *core.RecordDeleteEvent) error {
			//Cascade delete the collection
			fmt.Printf("A %s RRD was deleted: %s\n", cfg.ConfigCollection, e.Record.GetString(cfg.RingName))
			return nil
		})
		app.OnRecordAfterCreateRequest(cfg.ConfigCollection).Add(func(e *core.RecordCreateEvent) error {
			fmt.Printf("A %s RRD was created: %s\n", cfg.ConfigCollection, e.Record.GetString(cfg.RingName))
			enforceRRDOnCollection(app, cfg, e.Record)
			return nil
		})
		app.OnRecordAfterUpdateRequest(cfg.ConfigCollection).Add(func(e *core.RecordUpdateEvent) error {
			return nil
		})
		var records []*models.Record
		if err := app.Dao().RecordQuery(rrdCfgColl.Id).All(&records); err != nil {
			return err
		} else {
			for _, record := range records {
				id, unsub := enforceRRDOnCollection(app, cfg, record)
				unsubByCollection[id] = unsub
			}
		}
	}
	return nil
}
