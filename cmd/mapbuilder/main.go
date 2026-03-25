package main

import (
	"bufio"
	"fmt"
	"os"

	"hack2026mart/internal/mapedit"
)

func main() {
	fmt.Println("Простой мапбилдер JSON (для JSON_MAP_PATH в room).")
	fmt.Println("В конце — done или export [файл.json]; quit — выход без сохранения.")
	b, err := mapedit.RunInteractive(bufio.NewReader(os.Stdin), os.Stdout)
	if err != nil {
		if err == mapedit.ErrAborted {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := "custom_map.json"
	if len(os.Args) >= 2 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "запись:", err)
		os.Exit(1)
	}
	fmt.Printf("\nЗаписано: %s\n", out)
}
