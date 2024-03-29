package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/jinzhu/configor"
)

var (
	//go:embed ash.config.json
	defaultCfg string
	//go:embed res/vsdbg.sh
	vsdbgsh string
	//go:embed res/template.for.sshconfig.tmpl
	templateForSshconfig        string
	version, sha1ver, buildTime string
	cfg                         struct {
		Profiles                                             []string
		AwsInstances, KeysPath, SSHConfig, SSHConfigTemplate string
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
	var (
		out *os.File
		err error
	)
	if len(*oFlag) > 0 {
		if out, err = os.Create(*oFlag); err != nil {
			panic(err)
		}

	} else {
		if out, err = os.Create(cfg.SSHConfig); err != nil {
			panic(err)
		}
	}
	defer out.Close()
	var entries []string
	for _, p := range cfg.Profiles {
		for _, i := range Instances(p, cfg.KeysPath) {
			entries = append(entries, entry(i))
		}
	}
	cleanupHistory(entries)
	tmpl, err := template.ParseFiles(cfg.SSHConfigTemplate)
	if err != nil {
		panic(err)
	}
	if err := tmpl.Execute(out, entries); err != nil {
		panic(err)
	}
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
	if res == "q" || res == ":q" || res == "exit" || res == "quit" {
		os.Exit(0)
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

func contains(arr []string, s string) bool {
	for _, x := range arr {
		if s == x {
			return true
		}
	}
	return false
}

func getServer() *Server {
	history := loadHistory()
	f, _ := os.ReadFile(cfg.SSHConfig)
	allProfiles := regexp.MustCompile(`(?smU)# generated \[(.*)\].*$`).FindAllStringSubmatch(string(f), -1)
	profiles := append([]string{`history`, `all`}, distinct(elementsAt(allProfiles, 1))...)
	profile := ""
	srvname := ""
	if len(flag.Args()) == 1 && !contains(profiles, flag.Arg(0)) {
		profile = "all"
		srvname = flag.Arg(0)
	} else if len(flag.Args()) == 2 {
		profile = flag.Arg(0)
		srvname = flag.Arg(1)
	}
	i, profile, _ := inputStrings("> ", profiles, profile)
	entriesrxstr := `(?smU)# generated \[` + profile + `\].*\r?$.*Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	if i < 2 {
		entriesrxstr = `(?smU)Host (.*)\r?$.*HostName (.*)\r?$.*User (.*)\r?$.*IdentityFile (.*)\r?$`
	}
	entries := regexp.MustCompile(entriesrxstr).FindAllStringSubmatch(string(f), -1)
	if len(entries) > 0 && entries[0][1] == "*" {
		entries = entries[1:]
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i][1] < entries[j][1]
	})
	val := srvname
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
	fmt.Println("Paths:", []string{*cfgFlag, historyPath, cfg.SSHConfigTemplate})
	fmt.Println("Configuration:", fmt.Sprintf("%+v", cfg))
}

func serverInfo() {
	s := getServer()
	out := os.Stdout
	if len(*oFlag) > 0 {
		out, _ = os.Create(*oFlag)
		defer out.Close()
	}
	if *iaddressFlag {
		ip := strings.Split(s.Address, "@")[1]
		out.Write([]byte(ip))
		return
	}
	str, _ := json.Marshal(s)
	out.Write(str)
}

func ssh() {
	s := getServer()
	if s == nil {
		return
	}
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

var updateFlag = flag.Bool("u", false, "update ssh config file (path in config or specified by -o)")
var oFlag = flag.String("o", "", "output file")
var cfgFlag = flag.String("config-file ", "ash.config.json", "ash config file")
var putFlag = flag.String("put", "", "put file or directory")
var getFlag = flag.String("get", "", "get file or directory")
var execFlag = flag.String("exec", "", "execute command")
var versionFlag = flag.Bool("v", false, `print version`)
var serverFlag = flag.Bool("i", false, `outputs selected server info (use -o to print to file)`)
var iaddressFlag = flag.Bool("iaddress", false, `outputs selected server ip address (use -o to print to file)`)
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
	cfg.SSHConfigTemplate = lookForPath(cfg.SSHConfigTemplate, templateForSshconfig)
	home, _ := os.UserHomeDir()
	cfg.KeysPath = strings.ReplaceAll(cfg.KeysPath, "~", home)
	cfg.SSHConfig = strings.ReplaceAll(cfg.SSHConfig, "~", home)
	historyPath = lookForPath(historyPath, "")
	if *updateFlag || !fileExists(cfg.SSHConfig) {
		update()
		if len(flag.Args()) == 0 {
			return
		}
	}
	switch {
	case *versionFlag:
		info()
	case *vsdbgFlag:
		vsdbg()
	case *serverFlag, *iaddressFlag:
		serverInfo()
	default:
		ssh()
	}
}
