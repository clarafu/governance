package governance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Contributors map[string]Person
	Teams        map[string]Team
	Repos        map[string]Repo
}

type Person struct {
	Name    string            `yaml:"name"`
	GitHub  string            `yaml:"github"`
	Discord string            `yaml:"discord,omitempty"`
	Repos   map[string]string `yaml:"repos,omitempty"`
}

type Team struct {
	Name             string   `yaml:"name"`
	Purpose          string   `yaml:"purpose"`
	Responsibilities []string `yaml:"responsibilities"`

	Discord Discord `yaml:"discord,omitempty"`

	Members []string `yaml:"members"`

	Repos []string `yaml:"repos,omitempty"`

	// Legacy is set for teams that predated the governance model. These should
	// be eventually deleted.
	Legacy bool `yaml:"legacy,omitempty"`
}

type Discord struct {
	Role        string   `yaml:"role,omitempty"`
	Color       int      `yaml:"color,omitempty"`
	Priority    int      `yaml:"priority,omitempty"`
	Permissions []string `yaml:"permissions,omitempty"`
}

// 1. copied from https://discord.com/developers/docs/topics/permissions#permissions-bitwise-permission-flags
// 2. replaced GUILD with SERVER
var DiscordPermissions = map[string]int64{
	"CREATE_INSTANT_INVITE": 0x00000001,
	"KICK_MEMBERS":          0x00000002,
	"BAN_MEMBERS":           0x00000004,
	"ADMINISTRATOR":         0x00000008,
	"MANAGE_CHANNELS":       0x00000010,
	"MANAGE_SERVER":         0x00000020,
	"ADD_REACTIONS":         0x00000040,
	"VIEW_AUDIT_LOG":        0x00000080,
	"PRIORITY_SPEAKER":      0x00000100,
	"STREAM":                0x00000200,
	"VIEW_CHANNEL":          0x00000400,
	"SEND_MESSAGES":         0x00000800,
	"SEND_TTS_MESSAGES":     0x00001000,
	"MANAGE_MESSAGES":       0x00002000,
	"EMBED_LINKS":           0x00004000,
	"ATTACH_FILES":          0x00008000,
	"READ_MESSAGE_HISTORY":  0x00010000,
	"MENTION_EVERYONE":      0x00020000,
	"USE_EXTERNAL_EMOJIS":   0x00040000,
	"VIEW_SERVER_INSIGHTS":  0x00080000,
	"CONNECT":               0x00100000,
	"SPEAK":                 0x00200000,
	"MUTE_MEMBERS":          0x00400000,
	"DEAFEN_MEMBERS":        0x00800000,
	"MOVE_MEMBERS":          0x01000000,
	"USE_VAD":               0x02000000,
	"CHANGE_NICKNAME":       0x04000000,
	"MANAGE_NICKNAMES":      0x08000000,
	"MANAGE_ROLES":          0x10000000,
	"MANAGE_WEBHOOKS":       0x20000000,
	"MANAGE_EMOJIS":         0x40000000,
}

type Repo struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Private     bool       `yaml:"private,omitempty"`
	Topics      []string   `yaml:"topics,omitempty"`
	HomepageURL string     `yaml:"homepage_url,omitempty"`
	HasIssues   bool       `yaml:"has_issues,omitempty"`
	HasProjects bool       `yaml:"has_projects,omitempty"`
	HasWiki     bool       `yaml:"has_wiki,omitempty"`
	Pages       *RepoPages `yaml:"pages,omitempty"`
}

type RepoPages struct {
	CNAME  string `yaml:"cname,omitempty"`
	Branch string `yaml:"branch"`
	Path   string `yaml:"path,omitempty"`
}

func LoadConfig(tree fs.FS) (*Config, error) {
	personFiles, err := fs.ReadDir(tree, "contributors")
	if err != nil {
		return nil, err
	}

	contributors := map[string]Person{}
	for _, f := range personFiles {
		fn := filepath.Join("contributors", f.Name())

		file, err := tree.Open(fn)
		if err != nil {
			return nil, err
		}

		var person Person
		err = yaml.NewDecoder(file).Decode(&person)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", fn, err)
		}

		contributors[strings.TrimSuffix(f.Name(), ".yml")] = person
	}

	teamFiles, err := fs.ReadDir(tree, "teams")
	if err != nil {
		return nil, err
	}

	teams := map[string]Team{}
	for _, f := range teamFiles {
		fn := filepath.Join("teams", f.Name())

		file, err := tree.Open(fn)
		if err != nil {
			return nil, err
		}

		var team Team
		err = yaml.NewDecoder(file).Decode(&team)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", fn, err)
		}

		teams[strings.TrimSuffix(f.Name(), ".yml")] = team
	}

	repoFiles, err := fs.ReadDir(tree, "repos")
	if err != nil {
		return nil, err
	}

	repos := map[string]Repo{}
	for _, f := range repoFiles {
		fn := filepath.Join("repos", f.Name())

		file, err := tree.Open(fn)
		if err != nil {
			return nil, err
		}

		var repo Repo
		err = yaml.NewDecoder(file).Decode(&repo)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", fn, err)
		}

		repos[strings.TrimSuffix(f.Name(), ".yml")] = repo
	}

	return &Config{
		Contributors: contributors,
		Teams:        teams,
		Repos:        repos,
	}, nil
}

func (cfg Config) DesiredGitHubState() GitHubState {
	var state GitHubState

	repoCollaborators := map[string][]GitHubRepoCollaborator{}

	for _, person := range cfg.Contributors {
		state.Members = append(state.Members, GitHubOrgMember{
			Name:  person.Name,
			Login: person.GitHub,
			Role:  OrgRoleMember,
		})

		for repo, permission := range person.Repos {
			repoCollaborators[repo] = append(repoCollaborators[repo], GitHubRepoCollaborator{
				Login:      person.GitHub,
				Permission: permission3to4(permission),
			})
		}
	}

	for _, team := range cfg.Teams {
		if team.Legacy {
			continue
		}

		ghTeam := GitHubTeam{
			Name:        team.Name,
			Description: sanitize(team.Purpose),
		}

		for _, m := range team.Members {
			ghTeam.Members = append(ghTeam.Members, GitHubTeamMember{
				Login: m,
				Role:  TeamRoleMember,
			})
		}

		for _, r := range team.Repos {
			ghTeam.Repos = append(ghTeam.Repos, GitHubTeamRepoAccess{
				Name:       r,
				Permission: RepoPermissionMaintain,
			})
		}

		state.Teams = append(state.Teams, ghTeam)
	}

	for _, repo := range cfg.Repos {
		state.Repos = append(state.Repos, GitHubRepo{
			Name:                repo.Name,
			Description:         sanitize(repo.Description),
			IsPrivate:           repo.Private,
			Topics:              repo.Topics,
			HomepageURL:         repo.HomepageURL,
			HasIssues:           repo.HasIssues,
			HasProjects:         repo.HasProjects,
			HasWiki:             repo.HasWiki,
			DirectCollaborators: repoCollaborators[repo.Name],
		})
	}

	return state
}

func (config Config) SyncMissing(dest string) error {
	for name, person := range config.Contributors {
		filePath := filepath.Join(dest, "contributors", name+".yml")

		_, err := os.Stat(filePath)
		if err == nil {
			continue
		}

		if !os.IsNotExist(err) {
			return err
		}

		payload, err := yaml.Marshal(person)
		if err != nil {
			return err
		}

		fmt.Printf("syncing missing contributor: %s\n", name)

		err = os.WriteFile(filePath, payload, 0644)
		if err != nil {
			return err
		}
	}

	for name, team := range config.Teams {
		filePath := filepath.Join(dest, "teams", name+".yml")

		_, err := os.Stat(filePath)
		if err == nil {
			continue
		}

		if !os.IsNotExist(err) {
			return err
		}

		payload, err := yaml.Marshal(team)
		if err != nil {
			return err
		}

		fmt.Printf("syncing missing team: %s\n", name)

		err = os.WriteFile(filePath, payload, 0644)
		if err != nil {
			return err
		}
	}

	for name, repo := range config.Repos {
		filePath := filepath.Join(dest, "repos", name+".yml")

		_, err := os.Stat(filePath)
		if err == nil {
			continue
		}

		if !os.IsNotExist(err) {
			return err
		}

		payload, err := yaml.Marshal(repo)
		if err != nil {
			return err
		}

		fmt.Printf("syncing missing repo: %s\n", name)

		err = os.WriteFile(filePath, payload, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// collapse word-wrapped string YAML blocks
func sanitize(str string) string {
	return strings.TrimSpace(strings.Join(strings.Split(str, "\n"), " "))
}

func permission3to4(v3permission string) string {
	switch v3permission {
	case "pull":
		return "READ"
	case "push":
		return "WRITE"
	case "admin":
		return "ADMIN"
	case "maintain":
		return "MAINTAIN"
	case "triage":
		return "TRIAGE"
	default:
		return "INVALID"
	}
}

func permission4to3(v3permission string) string {
	switch v3permission {
	case "READ":
		return "pull"
	case "WRITE":
		return "push"
	case "ADMIN":
		return "admin"
	case "MAINTAIN":
		return "maintain"
	case "TRIAGE":
		return "triage"
	default:
		return "invalid"
	}
}
