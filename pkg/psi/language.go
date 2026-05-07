package psi

import (
    "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammars"
)

type LanguageInfo struct {
    Name       string
    Extensions []string
    GetLang    func() *gotreesitter.Language
}

var LanguageRegistry = map[string]*LanguageInfo{
    "go": {
        Name:       "Go",
        Extensions: []string{".go"},
        GetLang:    grammars.GoLanguage,
    },
    "python": {
        Name:       "Python",
        Extensions: []string{".py"},
        GetLang:    grammars.PythonLanguage,
    },
    "javascript": {
        Name:       "JavaScript",
        Extensions: []string{".js"},
        GetLang:    grammars.JavascriptLanguage,
    },
}

func DetectLanguage(filePath string) *LanguageInfo {
    for _, lang := range LanguageRegistry {
        for _, ext := range lang.Extensions {
            if len(filePath) > len(ext) && filePath[len(filePath)-len(ext):] == ext {
                return lang
            }
        }
    }
    return nil
}

func GetLanguage(name string) *LanguageInfo {
    return LanguageRegistry[name]
}