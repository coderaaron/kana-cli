package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/ChrisWiegman/kana-cli/internal/docker"
	"github.com/ChrisWiegman/kana-cli/internal/traefik"

	"github.com/docker/docker/api/types/mount"
)

type CurrentConfig struct {
	Type   string
	Local  bool
	Xdebug bool
}

type PluginInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Update  string `json:"update"`
	Version string `json:"version"`
}

// GetSiteContainers returns an array of strings containing the container names for the site
func (s *Site) GetSiteContainers() []string {

	return []string{
		fmt.Sprintf("kana_%s_database", s.StaticConfig.SiteName),
		fmt.Sprintf("kana_%s_wordpress", s.StaticConfig.SiteName),
	}
}

// IsSiteRunning Returns true if the site is up and running in Docker or false. Does not verify other errors
func (s *Site) IsSiteRunning() bool {

	containers, _ := s.dockerClient.ListContainers(s.StaticConfig.SiteName)

	return len(containers) != 0
}

// StopWordPress Stops the site in docker, destroying the containers when they close
func (s *Site) StopWordPress() error {

	wordPressContainers := s.GetSiteContainers()

	for _, wordPressContainer := range wordPressContainers {
		_, err := s.dockerClient.ContainerStop(wordPressContainer)
		if err != nil {
			return err
		}
	}

	// If no other sites are running, also shut down the Traefik container
	traefikClient, err := traefik.NewTraefik(s.StaticConfig)
	if err != nil {
		return err
	}

	return traefikClient.MaybeStopTraefik()
}

// getLocalAppDir Gets the absolute path to WordPress if the local flag or option has been set
func getLocalAppDir() (string, error) {

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	localAppDir := path.Join(cwd, "wordpress")

	err = os.MkdirAll(localAppDir, 0750)
	if err != nil {
		return "", err
	}

	return localAppDir, nil
}

func (s *Site) getMounts(appDir, siteType string) ([]mount.Mount, error) {

	appVolumes := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: appDir,
			Target: "/var/www/html",
		},
	}

	cwd, err := os.Getwd()
	if err != nil {
		return appVolumes, err
	}

	if siteType == "plugin" {
		appVolumes = append(appVolumes, mount.Mount{
			Type:   mount.TypeBind,
			Source: cwd,
			Target: path.Join("/var/www/html", "wp-content", "plugins", s.StaticConfig.SiteName),
		})
	}

	if siteType == "theme" {
		appVolumes = append(appVolumes, mount.Mount{
			Type:   mount.TypeBind,
			Source: cwd,
			Target: path.Join("/var/www/html", "wp-content", "themes", s.StaticConfig.SiteName),
		})
	}

	return appVolumes, nil
}

// StartWordPress Starts the WordPress containers
func (s *Site) StartWordPress() error {

	_, _, err := s.dockerClient.EnsureNetwork("kana")
	if err != nil {
		return err
	}

	appDir := path.Join(s.StaticConfig.SiteDirectory, "app")
	databaseDir := path.Join(s.StaticConfig.SiteDirectory, "database")

	if s.IsLocalSite() {
		appDir, err = getLocalAppDir()
		if err != nil {
			return err
		}

		// Replace wp-config.php with the container's file
		_, err := os.Stat(path.Join(appDir, "wp-config.php"))
		if err == nil {
			os.Remove(path.Join(appDir, "wp-config.php"))
		}
	}

	if err := os.MkdirAll(appDir, 0750); err != nil {
		return err
	}

	if err := os.MkdirAll(databaseDir, 0750); err != nil {
		return err
	}

	appVolumes, err := s.getMounts(appDir, s.SiteConfig.GetString("type"))
	if err != nil {
		return err
	}

	wordPressContainers := []docker.ContainerConfig{
		{
			Name:        fmt.Sprintf("kana_%s_database", s.StaticConfig.SiteName),
			Image:       "mariadb",
			NetworkName: "kana",
			HostName:    fmt.Sprintf("kana_%s_database", s.StaticConfig.SiteName),
			Env: []string{
				"MARIADB_ROOT_PASSWORD=password",
				"MARIADB_DATABASE=wordpress",
				"MARIADB_USER=wordpress",
				"MARIADB_PASSWORD=wordpress",
			},
			Labels: map[string]string{
				"kana.site": s.StaticConfig.SiteName,
			},
			Volumes: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: databaseDir,
					Target: "/var/lib/mysql",
				},
			},
		},
		{
			Name:        fmt.Sprintf("kana_%s_wordpress", s.StaticConfig.SiteName),
			Image:       fmt.Sprintf("wordpress:php%s", s.SiteConfig.GetString("php")),
			NetworkName: "kana",
			HostName:    fmt.Sprintf("kana_%s_wordpress", s.StaticConfig.SiteName),
			Env: []string{
				fmt.Sprintf("WORDPRESS_DB_HOST=kana_%s_database", s.StaticConfig.SiteName),
				"WORDPRESS_DB_USER=wordpress",
				"WORDPRESS_DB_PASSWORD=wordpress",
				"WORDPRESS_DB_NAME=wordpress",
			},
			Labels: map[string]string{
				"traefik.enable": "true",
				fmt.Sprintf("traefik.http.routers.wordpress-%s-http.entrypoints", s.StaticConfig.SiteName): "web",
				fmt.Sprintf("traefik.http.routers.wordpress-%s-http.rule", s.StaticConfig.SiteName):        fmt.Sprintf("Host(`%s.%s`)", s.StaticConfig.SiteName, s.StaticConfig.AppDomain),
				fmt.Sprintf("traefik.http.routers.wordpress-%s.entrypoints", s.StaticConfig.SiteName):      "websecure",
				fmt.Sprintf("traefik.http.routers.wordpress-%s.rule", s.StaticConfig.SiteName):             fmt.Sprintf("Host(`%s.%s`)", s.StaticConfig.SiteName, s.StaticConfig.AppDomain),
				fmt.Sprintf("traefik.http.routers.wordpress-%s.tls", s.StaticConfig.SiteName):              "true",
				"kana.site": s.StaticConfig.SiteName,
			},
			Volumes: appVolumes,
		},
	}

	for _, container := range wordPressContainers {

		err := s.dockerClient.EnsureImage(container.Image)
		if err != nil {
			return err
		}

		_, err = s.dockerClient.ContainerRun(container)
		if err != nil {
			return err
		}
	}

	return nil
}

// InstallWordPress Installs and configures WordPress core
func (s *Site) InstallWordPress() error {

	fmt.Println("Finishing WordPress setup...")

	setupCommand := []string{
		"core",
		"install",
		fmt.Sprintf("--url=%s", s.GetURL(false)),
		fmt.Sprintf("--title=Kana Development %s: %s", s.SiteConfig.GetString("type"), s.StaticConfig.SiteName),
		fmt.Sprintf("--admin_user=%s", s.DynamicConfig.GetString("admin.username")),
		fmt.Sprintf("--admin_password=%s", s.DynamicConfig.GetString("admin.password")),
		fmt.Sprintf("--admin_email=%s", s.DynamicConfig.GetString("admin.email")),
	}

	_, err := s.RunWPCli(setupCommand)
	return err
}

// InstallDefaultPlugins Installs a list of WordPress plugins
func (s *Site) InstallDefaultPlugins() error {

	for _, plugin := range s.SiteConfig.GetStringSlice("plugins") {

		setupCommand := []string{
			"plugin",
			"install",
			"--activate",
			plugin,
		}

		_, err := s.RunWPCli(setupCommand)
		if err != nil {
			return err
		}
	}

	return nil
}

// RunWPCli Runs a wp-cli command returning it's output and any errors
func (s *Site) RunWPCli(command []string) (string, error) {

	_, _, err := s.dockerClient.EnsureNetwork("kana")
	if err != nil {
		return "", err
	}

	siteDir := path.Join(s.StaticConfig.AppDirectory, "sites", s.StaticConfig.SiteName)
	appDir := path.Join(siteDir, "app")
	runningConfig := s.GetRunningConfig()

	if runningConfig.Local {
		appDir, err = getLocalAppDir()
		if err != nil {
			return "", err
		}
	}

	appVolumes, err := s.getMounts(appDir, runningConfig.Type)
	if err != nil {
		return "", err
	}

	fullCommand := []string{
		"wp",
		"--path=/var/www/html",
	}

	fullCommand = append(fullCommand, command...)

	container := docker.ContainerConfig{
		Name:        fmt.Sprintf("kana_%s_wordpress_cli", s.StaticConfig.SiteName),
		Image:       fmt.Sprintf("wordpress:cli-php%s", s.DynamicConfig.GetString("php")),
		NetworkName: "kana",
		HostName:    fmt.Sprintf("kana_%s_wordpress_cli", s.StaticConfig.SiteName),
		Command:     fullCommand,
		Env: []string{
			fmt.Sprintf("WORDPRESS_DB_HOST=kana_%s_database", s.StaticConfig.SiteName),
			"WORDPRESS_DB_USER=wordpress",
			"WORDPRESS_DB_PASSWORD=wordpress",
			"WORDPRESS_DB_NAME=wordpress",
		},
		Labels: map[string]string{
			"kana.site": s.StaticConfig.SiteName,
		},
		Volumes: appVolumes,
	}

	err = s.dockerClient.EnsureImage(container.Image)
	if err != nil {
		return "", err
	}

	_, output, err := s.dockerClient.ContainerRunAndClean(container)
	if err != nil {
		return "", err
	}

	return output, nil
}

// GetInstalledWordPressPlugins Returns a list of the plugins that have been installed on the site
func (s *Site) GetInstalledWordPressPlugins() ([]string, error) {

	commands := []string{
		"plugin",
		"list",
		"--format=json",
	}

	commandOutput, err := s.RunWPCli(commands)
	if err != nil {
		return []string{}, err
	}

	rawPlugins := []PluginInfo{}
	plugins := []string{}

	err = json.Unmarshal([]byte(commandOutput), &rawPlugins)
	if err != nil {
		return []string{}, err
	}

	for _, plugin := range rawPlugins {

		if plugin.Status != "dropin" && plugin.Name != s.StaticConfig.SiteName && plugin.Name != "hello" && plugin.Name != "akismet" {
			plugins = append(plugins, plugin.Name)
		}
	}

	return plugins, nil
}
