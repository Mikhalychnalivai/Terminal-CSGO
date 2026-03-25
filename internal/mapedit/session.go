// Package mapedit — интерактивный редактор простой сетки карты (JSON для jsonmap).
package mapedit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"hack2026mart/internal/game/jsonmap"
)

// ErrAborted — выход из редактора без сохранения (quit).
var ErrAborted = errors.New("редактор карт: отмена")

const (
	ansiRedFloor = "\033[41m"
	ansiGrayWall = "\033[100m"
	ansiSpawn    = "\033[42;97m" // зелёный фон — точка спавна
	ansiCursor   = "\033[43;30m"
	ansiReset    = "\033[0m"
)

// RunInteractive запускает цикл редактора: ввод/вывод — SSH или консоль.
// Команды: w a s d — курсор, t — стена/пол, p — точка спавна на полу, done|export — готово.
func RunInteractive(in *bufio.Reader, out io.Writer) ([]byte, error) {
	fmt.Fprintln(out, "Размер: 1 — стандарт 10×7, 2 — маленькая 8×6, 3 — большая 16×10 (Enter = 1)")
	fmt.Fprint(out, "> ")
	line, err := readLine(in, out)
	if err != nil {
		return nil, err
	}
	_, _ = io.WriteString(out, "\r\n")
	jw, jh := 10, 7
	switch strings.TrimSpace(line) {
	case "2":
		jw, jh = 8, 6
	case "3":
		jw, jh = 16, 10
	case "1", "":
	default:
		fmt.Fprintln(out, "Неизвестный выбор — стандарт 10×7.")
	}
	wall := newWallGrid(jw, jh)
	spawn := newSpawnGrid(jw, jh)
	cx, cy := 1, 1
	if wall[cy][cx] {
		cx, cy = jw/2, jh/2
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Красный «·» — пол, серый «#» — стена, зелёный «S» — спавн; рамка всегда стена.")
	fmt.Fprintln(out, "После каждой команды нажмите Enter. p — поставить/убрать спавн на полу под курсором.")
	fmt.Fprintln(out, "w/s/a/d — курсор, t — стена/пол, done — готово, quit — выход.")

	for {
		printGrid(out, wall, spawn, jw, jh, cx, cy)
		fmt.Fprint(out, "> ")
		ln, err := readLine(in, out)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrAborted
			}
			return nil, err
		}
		_, _ = io.WriteString(out, "\r\n")
		ln = normalizeArrowCommand(strings.TrimSpace(ln))
		if ln == "" {
			fmt.Fprintln(out, "  (пусто — введите w/a/s/d, t, p или done и Enter)")
			continue
		}
		parts := strings.Fields(ln)
		switch strings.ToLower(parts[0]) {
		case "quit", "q", "выход":
			return nil, ErrAborted

		case "done", "готово":
			return buildMapJSON(jw, jh, wall, spawn, "arena")

		case "export", "save", "сохранить":
			title := "custom"
			path := ""
			if len(parts) >= 2 {
				path = strings.Join(parts[1:], " ")
				base := filepath.Base(path)
				title = strings.TrimSuffix(base, filepath.Ext(base))
			}
			b, err := buildMapJSON(jw, jh, wall, spawn, title)
			if err != nil {
				return nil, err
			}
			if path != "" {
				if err := os.WriteFile(path, b, 0644); err != nil {
					fmt.Fprintln(out, "ошибка записи:", err)
					continue
				}
				fmt.Fprintf(out, "Записано: %s\n", path)
			}
			return b, nil

		case "t", "toggle", "п":
			if cx > 0 && cx < jw-1 && cy > 0 && cy < jh-1 {
				wall[cy][cx] = !wall[cy][cx]
				if wall[cy][cx] {
					spawn[cy][cx] = false
				}
				enforceBorder(wall, spawn, jw, jh)
			}
		case "p", "спавн":
			if cx > 0 && cx < jw-1 && cy > 0 && cy < jh-1 && !wall[cy][cx] {
				spawn[cy][cx] = !spawn[cy][cx]
			}
		case "w":
			if cy > 1 {
				cy--
			}
		case "s":
			if cy < jh-2 {
				cy++
			}
		case "a":
			if cx > 1 {
				cx--
			}
		case "d":
			if cx < jw-2 {
				cx++
			}
		default:
			fmt.Fprintln(out, "Неизвестно: w s a d — курсор, t — стена, p — спавн, done — готово.")
		}
	}
}

// normalizeArrowCommand — если в строке есть код стрелки (SSH часто даёт ESC [ A и т.д.), сводим к одной букве.
func normalizeArrowCommand(ln string) string {
	if ln == "" {
		return ""
	}
	switch {
	case strings.Contains(ln, "\x1b[A") || strings.Contains(ln, "\x1bOA"):
		return "w"
	case strings.Contains(ln, "\x1b[B") || strings.Contains(ln, "\x1bOB"):
		return "s"
	case strings.Contains(ln, "\x1b[C") || strings.Contains(ln, "\x1bOC"):
		return "d"
	case strings.Contains(ln, "\x1b[D") || strings.Contains(ln, "\x1bOD"):
		return "a"
	default:
		return ln
	}
}

func buildMapJSON(jw, jh int, wall, spawn [][]bool, title string) ([]byte, error) {
	f, err := jsonmap.BuildSimpleWallFile(jw, jh, wall, spawn, title)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(f, "", "  ")
}

// readLine — как в gateway: эхо в SSH и конец строки по \n или \r (часто только CR).
func readLine(r *bufio.Reader, out io.Writer) (string, error) {
	skipLeadingLineNoise(r)
	var buf []byte
	for {
		ch, err := r.ReadByte()
		if err != nil {
			if err == io.EOF && len(buf) == 0 {
				return "", err
			}
			if err == io.EOF {
				return strings.TrimSpace(string(buf)), io.EOF
			}
			return "", err
		}
		if ch == '\n' {
			break
		}
		if ch == '\r' {
			if r.Buffered() > 0 {
				if peek, err := r.Peek(1); err == nil && len(peek) > 0 && peek[0] == '\n' {
					_, _ = r.ReadByte()
				}
			}
			break
		}
		if ch == 8 || ch == 127 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				if out != nil {
					_, _ = io.WriteString(out, "\b \b")
				}
			}
			continue
		}
		if out != nil && (ch >= 32 || ch > 127) {
			_, _ = out.Write([]byte{ch})
		}
		buf = append(buf, ch)
	}
	return strings.TrimSpace(string(buf)), nil
}

func skipLeadingLineNoise(r *bufio.Reader) {
	for r.Buffered() > 0 {
		peek, err := r.Peek(1)
		if err != nil || len(peek) == 0 {
			return
		}
		c := peek[0]
		if c != '\n' && c != '\r' {
			return
		}
		_, _ = r.ReadByte()
		if c == '\r' && r.Buffered() > 0 {
			p2, _ := r.Peek(1)
			if len(p2) > 0 && p2[0] == '\n' {
				_, _ = r.ReadByte()
			}
		}
	}
}

func newWallGrid(jw, jh int) [][]bool {
	g := make([][]bool, jh)
	for y := 0; y < jh; y++ {
		g[y] = make([]bool, jw)
		for x := 0; x < jw; x++ {
			g[y][x] = x == 0 || x == jw-1 || y == 0 || y == jh-1
		}
	}
	return g
}

func newSpawnGrid(jw, jh int) [][]bool {
	g := make([][]bool, jh)
	for y := 0; y < jh; y++ {
		g[y] = make([]bool, jw)
	}
	return g
}

func enforceBorder(wall, spawn [][]bool, jw, jh int) {
	for y := 0; y < jh; y++ {
		for x := 0; x < jw; x++ {
			if x == 0 || x == jw-1 || y == 0 || y == jh-1 {
				wall[y][x] = true
				spawn[y][x] = false
			}
		}
	}
}

func printGrid(out io.Writer, wall, spawn [][]bool, jw, jh, cx, cy int) {
	fmt.Fprint(out, "\033[2J\033[H")
	fmt.Fprintf(out, "Размер %d×%d | курсор (%d,%d) | p=спавн, t=стена\n\n", jw, jh, cx, cy)
	for y := 0; y < jh; y++ {
		fmt.Fprintf(out, "%2d ", y)
		for x := 0; x < jw; x++ {
			if x == cx && y == cy {
				ch := "·"
				if wall[y][x] {
					ch = "#"
				} else if spawn[y][x] {
					ch = "S"
				}
				fmt.Fprint(out, ansiCursor+ch+ansiReset)
				continue
			}
			if wall[y][x] {
				fmt.Fprint(out, ansiGrayWall+"#"+ansiReset)
			} else if spawn[y][x] {
				fmt.Fprint(out, ansiSpawn+"S"+ansiReset)
			} else {
				fmt.Fprint(out, ansiRedFloor+"·"+ansiReset)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out)
}
