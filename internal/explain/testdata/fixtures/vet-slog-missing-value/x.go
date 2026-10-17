package fixture

import "log/slog"

func LogRequest(path string, status int) {
	slog.Info("request", "path", path, "status")
}
