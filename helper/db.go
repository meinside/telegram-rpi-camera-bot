package helper

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// constants for local database
	DbFilename = "db.sqlite"
)

type Database struct {
	db *sql.DB
	sync.RWMutex
}

type Photo struct {
	UserName string
	FileId   string
	Caption  string
	Time     time.Time
}

var _db *Database = nil

func OpenDb() *Database {
	if _db == nil {
		if execFilepath, err := os.Executable(); err != nil {
			panic(err)
		} else {
			if db, err := sql.Open("sqlite3", filepath.Join(filepath.Dir(execFilepath), DbFilename)); err != nil {
				panic("Failed to open database: " + err.Error())
			} else {
				_db = &Database{
					db: db,
				}

				// photos table
				if _, err := db.Exec(`create table if not exists photos(
					id integer primary key autoincrement,
					user_name text not null,
					file_id text not null,
					caption text default null,
					time datetime default current_timestamp
				)`); err != nil {
					panic("Failed to create photos table: " + err.Error())
				}
				if _, err := db.Exec(`create index if not exists idx_photos on photos(
					user_name,
					time
				)`); err != nil {
					panic("Failed to create photos table: " + err.Error())
				}
			}
		}
	}

	return _db
}

func CloseDb() {
	if _db != nil {
		_db.db.Close()
		_db = nil
	}
}

func (d *Database) SavePhoto(userName, fileId, caption string) {
	d.Lock()

	if stmt, err := d.db.Prepare(`insert into photos(user_name, file_id, caption) values(?, ?, ?)`); err != nil {
		log.Printf("*** Failed to prepare a statement: %s\n", err.Error())
	} else {
		defer stmt.Close()
		if _, err = stmt.Exec(userName, fileId, caption); err != nil {
			log.Printf("*** Failed to save photo into local database: %s\n", err.Error())
		}
	}

	d.Unlock()
}

func (d *Database) GetPhotos(userName string, latestN int) []Photo {
	photos := []Photo{}

	d.RLock()

	if stmt, err := d.db.Prepare(`select user_name, file_id, caption, datetime(time, 'localtime') as time from photos where user_name = ? order by id desc limit ?`); err != nil {
		log.Printf("*** Failed to prepare a statement: %s\n", err.Error())
	} else {
		defer stmt.Close()

		if rows, err := stmt.Query(userName, latestN); err != nil {
			log.Printf("*** Failed to select photos from local database: %s\n", err.Error())
		} else {
			defer rows.Close()

			var userName, fileId, caption, datetime string
			var tm time.Time
			for rows.Next() {
				rows.Scan(&userName, &fileId, &caption, &datetime)
				tm, _ = time.Parse("2006-01-02 15:04:05", datetime)

				photos = append(photos, Photo{
					UserName: userName,
					FileId:   fileId,
					Caption:  caption,
					Time:     tm,
				})
			}
		}
	}

	d.RUnlock()

	return photos
}
