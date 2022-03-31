package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/jinzhu/configor"
)

var (
	//go:embed ash.config.json
	defaultCfg string
	//go:embed res/vsdbg.sh
	vsdbgsh                     string
	version, sha1ver, buildTime string
	cfg                         struct {
		Profiles                                    []string
		AwsInstances, KeysPath, SSHConfig, AppendTo string
	}
)

func elementsAt[K any](in [][]K, idx int) []K {
	var res []K
	for _, v := range in {
		res = append(res, v[idx])
	}
	return res
}

func distinct[K comparable](in []K) []K {
	keys := make(map[K]bool)
	out := []K{}
	for _, entry := range in {
		if _, ok := keys[entry]; !ok {
			keys[entry] = true
			out = append(out, entry)
		}
	}
	return out
}

func entry(s Server) string {
	user := "ubuntu"
	if s.Platform == "windows" {
		user = "administrator"
	}
	return fmt.Sprintf(`# generated [%s]
Host %s
    HostName %s
    User %s
    IdentityFile %s
`, s.Profile, strings.ReplaceAll(s.Name, " ", ""), s.Address, user, s.Key)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func lookForPath(filePath, defaultContent string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	exePath, _ := os.Executable()
	exePath = filepath.Dir(exePath)
	cwdPath, _ := os.Getwd()
	homePath, _ := os.UserHomeDir()
	homePath = path.Join(homePath, ".config/ash")
	for _, dir := range []string{exePath, cwdPath, homePath} {
		p := path.Join(dir, filePath)
		if fileExists(p) {
			return p
		}
	}
	defaultPath := path.Join(homePath, filePath)
	_ = os.MkdirAll(filepath.Dir(defaultPath), os.ModePerm)
	_ = os.WriteFile(defaultPath, []byte(defaultContent), os.ModePerm)
	return defaultPath
}

func loadHistory() []string {
	if !fileExists(historyPath) {
		os.WriteFile(historyPath, []byte(`[]`), os.ModeAppend)
	}
	historyFile, _ := os.ReadFile(historyPath)
	var history []string
	json.Unmarshal(historyFile, &history)
	return history
}

func saveHistory(history []string) {
	b, _ := json.Marshal(history)
	_ = os.WriteFile(historyPath, b, os.ModeAppend)
}

func cleanupHistory(entries []string) {
	var cleanedUpHistory []string
Next:
	for _, h := range loadHistory() {
		for _, e := range entries {
			if e == h {
				cleanedUpHistory = append(cleanedUpHistory, h)
				continue Next
			}
		}
	}
	saveHistory(cleanedUpHistory)
}

func update() {
	out := os.Stdout
	if !*printFlag {
		if *oFlag != "out" {
			out, _ = os.Create(*oFlag)
		} else {
			out, _ = os.Create(cfg.SSHConfig)
		}
		defer out.Close()
	}
	if appendTo, err := os.ReadFile(cfg.AppendTo); err == nil {
		out.Write(appendTo)
	}
	var entries []string
	for _, p := range cfg.Profiles {
		for _, i := range Instances(p, cfg.KeysPath) {
			fmt.Fprintf(out, "%s\n", entry(i))
			entries = append(entries, strings.ReplaceAll(i.Name, " ", ""))
		}
	}
	cleanupHistory(entries)
}

func inputStrings(prefix string, values []string, in ...string) (int, string, error) {
	suggests := make([]prompt.Suggest, len(values))
	for i, v := range values {
		suggests[i] = prompt.Suggest{Text: v}
	}
	return inputSuggests(prefix, suggests, in...)
}

func inputSuggests(prefix string, suggests []prompt.Suggest, in ...string) (int, string, error) {
	completer := func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
	var res string
	if len(in) > 0 && in[0] != "" {
		res = in[0]
	} else {
		res = prompt.Input(prefix, completer, prompt.OptionShowCompletionAtStart(), prompt.OptionCompletionOnDown(), prompt.OptionSuggestionBGColor(prompt.DarkGray))
	}
	var selected []prompt.Suggest
	for _, e := range suggests {
		if e.Text == res {
			selected = append(selected, e)
		}
	}
	if len(selected) == 0 {
		selected = prompt.FilterFuzzy(suggests, res, true)
	}
	if len(selected) == 0 {
		return -1, "", errors.New("can't find " + res)
	}
	for i, s := range suggests {
		if selected[0] == s {
			return i, s.Text, nil
		}
	}
	return -1, "", errors.New("can't find " + res)
}

func getServer() *Server {
	history := loadHistory()
	f, _ := os.ReadFile(cfg.SSHConfig)
	allProfiles := regexp.MustCompile(`(?smU)# generated \[(.*)\].*$`).FindAllStringSubmatch(string(f), -1)
	profiles := append([]string{`history`, `all`}, distinct(elementsAt(allProfiles, 1))...)
	i, profile, _ := inputStrings("> ", profiles, flag.Arg(0))
	entriesrxstr := `(?smU)# generated \[` + profile + `\].*\r?$.*Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	if i < 2 {
		entriesrxstr = `(?smU)Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	}
	entries := regexp.MustCompile(entriesrxstr).FindAllStringSubmatch(string(f), -1)
	val := flag.Arg(1)
	if profile == `history` {
		_, val, _ = inputStrings(profile+"> ", history)
	}
	i, _, err := inputStrings(profile+"> ", elementsAt(entries, 1), val)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	srv := &Server{entries[i][1], entries[i][3] + "@" + entries[i][2], profile, entries[i][4], ""}
	history = distinct(append([]string{srv.Name}, history...))
	saveHistory(history)
	return srv
}

func info() {
	fmt.Println("Version:", version)
	fmt.Println("Build:", sha1ver, buildTime)
	fmt.Println("Paths:", []string{*cfgFlag, historyPath, cfg.AppendTo})
	fmt.Println("Configuration:", fmt.Sprintf("%+v", cfg))
}

func serverInfo() {
	s := getServer()
	out := os.Stdout
	if !*printFlag {
		out, _ = os.Create(*oFlag)
		defer out.Close()
	}
	str, _ := json.Marshal(s)
	out.Write(str)
}

func ssh() {
	s := getServer()
	switch {
	case *putFlag != "":
		executeInteractive(`scp`, `-i`, s.Key, *putFlag, s.Address+`:`)
	case *getFlag != "":
		executeInteractive(`scp`, `-i`, s.Key, s.Address+`:`+*getFlag, `.`)
	case *execFlag != "":
		executeInteractive(`ssh`, `-i`, s.Key, s.Address, *execFlag)
	default:
		executeInteractive(`ssh`, `-i`, s.Key, s.Address)
	}
}

func execute(name string, arg ...string) string {
	fmt.Println("execute", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdin = strings.NewReader("mah")
	o, _ := cmd.CombinedOutput()
	return string(o)
}

func executeInteractive(name string, arg ...string) {
	fmt.Println("execute", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func vsdbg() {
	s := getServer()
	ls := execute(`ssh`, `-i`, s.Key, s.Address, `sudo docker ps --format "{{.ID}},{{.Names}},{{.Image}}"`)
	os.WriteFile("tmp", []byte(ls), os.ModeAppend)
	containers := [][]string{}
	var cNames []prompt.Suggest
	for _, l := range strings.Split(ls, "\n") {
		x := strings.Split(l, ",")
		if len(x) > 1 {
			containers = append(containers, x)
			cNames = append(cNames, prompt.Suggest{Text: x[1], Description: x[2]})
		}
	}
	i, _, _ := inputSuggests("", cNames)
	executeInteractive(`scp`, `-i`, s.Key, lookForPath(`res/vsdbg.sh`, vsdbgsh), s.Address+`:`)
	executeInteractive(`ssh`, `-i`, s.Key, s.Address, `sudo`, `bash`, `vsdbg.sh`, containers[i][0], *vsdbgPortFlag)
	fmt.Println("SSH to", s.Address, *vsdbgPortFlag)
}

var updateFlag = flag.Bool("update", false, "update ssh config file (path in config or specified by -o)")
var oFlag = flag.String("o", "out", "output file to use when -print is not set")
var printFlag = flag.Bool("print", false, "print to stdout")
var cfgFlag = flag.String("config-file ", "ash.config.json", "ash config file")
var putFlag = flag.String("put", "", "put file or directory")
var getFlag = flag.String("get", "", "get file or directory")
var execFlag = flag.String("exec", "", "execute command")
var versionFlag = flag.Bool("version", false, `print version`)
var serverFlag = flag.Bool("server", false, `print selected server info to standard output if -print is set, to file otherwise`)
var vsdbgFlag = flag.Bool("vsdbg", false, `setup .net remote container debug`)
var vsdbgPortFlag = flag.String("vsdbgport", "4444", `.net remote container port`)
var historyPath = `history`

func main() {
	flag.Parse()
	lookForPath(`res/vsdbg.sh`, vsdbgsh)
	*cfgFlag = lookForPath(*cfgFlag, defaultCfg)
	if err := configor.Load(&cfg, *cfgFlag); err != nil {
		fmt.Println(err)
		return
	}
	cfg.AppendTo = lookForPath(cfg.AppendTo, "")
	cfg.KeysPath = strings.ReplaceAll(cfg.KeysPath, "%userprofile%", os.Getenv("userprofile"))
	cfg.SSHConfig = strings.ReplaceAll(cfg.SSHConfig, "%userprofile%", os.Getenv("userprofile"))
	historyPath = lookForPath(historyPath, "")
	switch {
	case *versionFlag:
		info()
	case *updateFlag, !fileExists(cfg.SSHConfig):
		update()
	case *vsdbgFlag:
		vsdbg()
	case *serverFlag:
		serverInfo()
	default:
		ssh()
	}
}
