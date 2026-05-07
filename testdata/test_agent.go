package testdata

type Agent struct {
    Name string
}

func (a *Agent) Think() {
    a.Analyze()
    result := a.Process("input")
    fmt.Println(result)
}

func (a *Agent) Analyze() string {
    a.validate()
    return "analyzed"
}

func (a *Agent) Process(input string) string {
    a.validate()
    return "processed: " + input
}

func (a *Agent) validate() bool {
    return true
}