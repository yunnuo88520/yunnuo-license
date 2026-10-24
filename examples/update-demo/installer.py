#!/usr/bin/env python3
"""Small installer for this demo's two-file package and three known SQLite schemas."""
import json
import os
import pathlib
import shutil
import sqlite3
import stat
import sys
import uuid
import zipfile


def unpack(archive, target, expected_version):
    with zipfile.ZipFile(archive) as package:
        entries = package.infolist()
        if len(entries) != 2 or {e.filename for e in entries} != {"index.html", "release.json"}:
            raise ValueError("unexpected package files or duplicate entries")
        if sum(e.file_size for e in entries) > 8 * 1024 * 1024:
            raise ValueError("expanded package exceeds demo limit")
        for entry in entries:
            mode = entry.external_attr >> 16
            if stat.S_ISLNK(mode) or entry.is_dir():
                raise ValueError("package links and directories are forbidden")
            (target / entry.filename).write_bytes(package.read(entry))
    metadata = json.loads((target / "release.json").read_text())
    if metadata["version"] != expected_version or type(metadata["schema_version"]) is not int or metadata["schema_version"] not in (1, 2, 3):
        raise ValueError("unexpected product version or schema")
    return metadata


def switch(base, target):
    link = base / (".switch-" + uuid.uuid4().hex)
    try:
        link.symlink_to(target)
        os.replace(link, base / "current")
    finally:
        link.unlink(missing_ok=True)


def rollback(base):
    pending = base / "pending.json"
    if not pending.exists():
        return {"rolled_back": False}
    transaction = json.loads(pending.read_text())
    with sqlite3.connect(transaction["backup"]) as source, sqlite3.connect(base / "site.sqlite") as target:
        source.backup(target)
    switch(base, transaction["old_target"])
    pending.unlink()
    return {"rolled_back": True}


def main():
    action, raw_base, *args = sys.argv[1:]
    base = pathlib.Path(raw_base).resolve()
    if action == "init":
        base.mkdir(parents=True, exist_ok=True)
        if (base / "site.sqlite").exists() or (base / "current").exists():
            raise ValueError("refusing to overwrite an existing demo")
        target = base / "releases" / "initial"
        target.mkdir(parents=True)
        unpack(args[0], target, "1.0.0")
        with sqlite3.connect(base / "site.sqlite") as db:
            db.execute("CREATE TABLE notes (id INTEGER PRIMARY KEY, title TEXT NOT NULL)")
            db.execute("INSERT INTO notes (title) VALUES (?)", ("客户已有文章：升级后仍应完整保留",))
            db.execute("PRAGMA user_version = 1")
        switch(base, str(target))
        return {"initialized": True}
    if action == "status":
        with sqlite3.connect(base / "site.sqlite") as db:
            schema = db.execute("PRAGMA user_version").fetchone()[0]
            rows = db.execute("SELECT id, title FROM notes ORDER BY id").fetchall()
            tables = [row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")]
        return {"schema_version": schema, "notes": [{"id": r[0], "title": r[1]} for r in rows], "tables": tables}
    if action == "apply":
        if (base / "pending.json").exists():
            raise ValueError("an earlier update must be recovered first")
        target = base / "releases" / uuid.uuid4().hex
        target.mkdir(parents=True)
        try:
            metadata = unpack(args[0], target, args[1])
            backup = base / "backups" / (uuid.uuid4().hex + ".sqlite")
            backup.parent.mkdir(exist_ok=True)
            with sqlite3.connect(base / "site.sqlite") as db, sqlite3.connect(backup) as snapshot:
                db.backup(snapshot)
            transaction = {"old_target": str((base / "current").resolve(strict=True)), "backup": str(backup)}
            (base / "pending.json").write_text(json.dumps(transaction))
            with sqlite3.connect(base / "site.sqlite") as db:
                schema = db.execute("PRAGMA user_version").fetchone()[0]
                wanted = metadata["schema_version"]
                if wanted < schema:
                    raise ValueError("database downgrade is not supported")
                db.execute("BEGIN IMMEDIATE")
                if schema < 2 <= wanted:
                    db.execute("ALTER TABLE notes ADD COLUMN tag TEXT NOT NULL DEFAULT 'preserved'")
                if schema < 3 <= wanted:
                    db.execute("CREATE TABLE version_three_only (id INTEGER PRIMARY KEY)")
                db.execute("PRAGMA user_version = " + str(wanted))
            switch(base, str(target))
            return {"applied": True, "version": metadata["version"]}
        except Exception:
            rollback(base)
            shutil.rmtree(target)
            raise
    if action == "rollback":
        return rollback(base)
    if action == "commit":
        (base / "pending.json").unlink()
        return {"committed": True}
    raise ValueError("unknown installer command")


if __name__ == "__main__":
    print(json.dumps(main(), ensure_ascii=False))
