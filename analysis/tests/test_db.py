import db


def test_read_env_file(tmp_path):
    path = tmp_path / ".env"
    path.write_text("# comment\n\nPOSTGRES_USER=vetcare\nPOSTGRES_PASSWORD='secret'\nBROKEN\n")
    assert db.read_env_file(path) == {"POSTGRES_USER": "vetcare", "POSTGRES_PASSWORD": "secret"}


def test_read_env_file_missing(tmp_path):
    assert db.read_env_file(tmp_path / "nope") == {}


def test_environment_overrides_env_file(monkeypatch):
    monkeypatch.setenv("PGHOST", "db.example")
    monkeypatch.setenv("PGUSER", "ci")
    params = db.connection_params()
    assert params["host"] == "db.example"
    assert params["user"] == "ci"
