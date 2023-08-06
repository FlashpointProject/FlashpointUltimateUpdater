package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type SqliteRepo struct {
	db *sql.DB
}

func OpenDatabase(filepath string) (*SqliteRepo, error) {
	db, err := sql.Open("sqlite3", filepath+"?cache=shared")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// Clear all taken markets
	_, err = db.Exec("UPDATE files SET taken = false WHERE taken = true")
	if err != nil {
		return nil, err
	}

	return &SqliteRepo{
		db,
	}, nil
}

func (repo *SqliteRepo) GetOverview() (IndexOverview, error) {
	var overview IndexOverview
	err := repo.db.QueryRow("SELECT name, total_files, total_size, base_url FROM overview LIMIT 1").
		Scan(&overview.Name, &overview.TotalFiles, &overview.TotalSize, &overview.BaseUrl)
	if err != nil {
		return overview, err
	}
	return overview, nil
}

func (repo *SqliteRepo) GetTotalDownloadedSize() (int64, error) {
	var total int64
	err := repo.db.QueryRow("SELECT IFNULL(sum(size), 0) FROM files WHERE done = true").
		Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (repo *SqliteRepo) MarkFileDone(file *IndexedFile) error {
	_, err := repo.db.Exec("UPDATE files SET done = true WHERE path = ?", file.Filepath)
	return err
}

func (repo *SqliteRepo) GetNextFile() (*IndexedFile, error) {
	var f IndexedFile
	err := repo.db.QueryRow(`UPDATE files SET taken = true
		WHERE rowid = (
		    SELECT MIN(rowid)
		    FROM files
		    WHERE done = false AND taken = false
		) RETURNING path, size, sha1`).Scan(&f.Filepath, &f.Size, &f.SHA1)
	fmt.Println(fmt.Sprintf("NEXT path: %s, size: %d", f.Filepath, f.Size))
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (repo *SqliteRepo) GetNextFileBatch(limit int64) ([]*IndexedFile, error) {
	rows, err := repo.db.Query("SELECT path, size, sha1 FROM files WHERE done = false AND taken = false LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]*IndexedFile, 0)
	for rows.Next() {
		var f IndexedFile
		err = rows.Scan(&f.Filepath, &f.Size, &f.SHA1)
		if err != nil {
			return nil, err
		}
		files = append(files, &f)
	}

	rows.Close()

	for _, f := range files {
		_, updateErr := repo.db.Exec("UPDATE files SET taken = true WHERE path = ?", f.Filepath)
		if updateErr != nil {
			// Handle the update error if needed
			return nil, updateErr
		}
	}
	return files, nil
}

func (repo *SqliteRepo) ResetDownloadState() error {
	_, err := repo.db.Exec("UPDATE files SET done = false")
	if err != nil {
		return err
	}
	_, err = repo.db.Exec("UPDATE files SET taken = false")
	return err
}