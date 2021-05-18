package gitea

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	giteaSDK "code.gitea.io/sdk/gitea"
	"github.com/epinio/epinio/internal/api/v1/models"
	"github.com/pkg/errors"
)

const LocalRegistry = "127.0.0.1:30500/apps"

// Upload puts the app data into the gitea repo and creates the webhook and
// accompanying app data.
// The results are added to the struct App.
func (c *Client) Upload(app *models.App, tmpDir string) error {
	org := app.Org
	name := app.Name

	err := c.createRepo(org, name)
	if err != nil {
		return errors.Wrap(err, "failed to create application")
	}

	app.Route = c.AppDefaultRoute(name)

	// sets repo.url, imageID
	err = c.prepareCode(app, tmpDir)
	if err != nil {
		return err
	}

	// sets repo.revision
	err = c.gitPush(app, tmpDir)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) AppDefaultRoute(name string) string {
	return fmt.Sprintf("%s.%s", name, c.Domain)
}

func (c *Client) createRepo(org string, name string) error {
	_, resp, err := c.Client.GetRepo(org, name)
	if resp == nil && err != nil {
		return errors.Wrap(err, "failed to make get repo request")
	}

	if resp.StatusCode == 200 {
		return nil
	}

	_, _, err = c.Client.CreateOrgRepo(org, giteaSDK.CreateRepoOption{
		Name:          name,
		AutoInit:      true,
		Private:       true,
		DefaultBranch: "main",
	})

	if err != nil {
		return errors.Wrap(err, "failed to create application")
	}

	return nil
}

// prepareCode - add the deployment info files
func (c *Client) prepareCode(app *models.App, tmpDir string) error {
	err := os.MkdirAll(filepath.Join(tmpDir, ".kube"), 0700)
	if err != nil {
		return errors.Wrap(err, "failed to setup kube resources directory in temp app location")
	}

	err = c.renderDeployment(filepath.Join(tmpDir, ".kube", "app.yml"), app)
	if err != nil {
		return err
	}

	if err := renderService(filepath.Join(tmpDir, ".kube", "service.yml"), app.Org, app.Name); err != nil {
		return err
	}

	if err := renderIngress(filepath.Join(tmpDir, ".kube", "ingress.yml"), app.Org, app.Name, app.Route); err != nil {
		return err
	}

	return nil
}

// gitPush the app data
func (c *Client) gitPush(app *models.App, tmpDir string) error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return errors.Wrap(err, "failed to parse gitea url")
	}

	u.User = url.UserPassword(c.Username, c.Password)
	u.Path = path.Join(u.Path, app.Org, app.Name)

	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf(`
cd "%s" 
git init
git config user.name "Epinio"
git config user.email ci@epinio
git remote add epinio "%s"
git fetch --all
git reset --soft epinio/main
git add --all
git commit -m "pushed at %s"
git push epinio %s:main
`, tmpDir, u.String(), time.Now().Format("20060102150405"), "`git branch --show-current`"))

	_, err = cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "push script failed")
	}

	// extract commit sha
	cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf(`
cd "%s"
git rev-parse HEAD
`, tmpDir))

	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "failed to determine last commit")
	}

	// SHA of second commit
	app.Git.Revision = strings.TrimSuffix(string(out), "\n")
	return nil
}

func (c *Client) renderDeployment(filePath string, app *models.App) error {
	deploymentTmpl, err := template.New("deployment").Parse(`
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: "{{ .AppName }}"
  namespace: {{ .Org }}
  labels:
    app.kubernetes.io/name: "{{ .AppName }}"
    app.kubernetes.io/part-of: "{{ .Org }}"
    app.kubernetes.io/component: application
    app.kubernetes.io/managed-by: epinio
spec:
  replicas: {{ .Instances }}
  selector:
    matchLabels:
      app.kubernetes.io/name: "{{ .AppName }}"
  template:
    metadata:
      labels:
        app.kubernetes.io/name: "{{ .AppName }}"
        epinio.suse.org/image-id: "{{ .ImageID }}"
        epinio.suse.org/stage-id: "{{ .StageID }}"
        app.kubernetes.io/part-of: "{{ .Org }}"
        app.kubernetes.io/component: application
        app.kubernetes.io/managed-by: epinio
      annotations:
        app.kubernetes.io/name: "{{ .AppName }}"
    spec:
      # TODO: Do these when you create an org
      serviceAccountName: {{ .Org }}
      automountServiceAccountToken: false
      containers:
      - name: "{{ .AppName }}"
        image: "{{ .Image }}"
        ports:
        - containerPort: 8080
        env:
        - name: PORT
          value: "8080"
  `)
	if err != nil {
		return errors.Wrap(err, "failed to parse deployment template for app")
	}

	appFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to create file for kube resource definitions")
	}
	defer func() { err = appFile.Close() }()
	commit, _, err := c.Client.GetSingleCommit(app.Org, app.Name, "HEAD")
	if err != nil {
		return errors.Wrap(err, "failed to get latest app commit")
	}

	// SHA of first commit, used in app.yml, which is part of second commit
	app.Image = models.NewImage(commit.RepoCommit.Tree.SHA[:8])
	app.Git = &models.GitRef{
		URL: c.URL,
	}

	err = deploymentTmpl.Execute(appFile, struct {
		AppName   string
		Route     string
		Org       string
		Image     string
		Instances int32
		ImageID   string
		StageID   string
	}{
		AppName:   app.Name,
		Route:     app.Route,
		Org:       app.Org,
		Image:     app.ImageURL(LocalRegistry),
		Instances: app.Instances,
		ImageID:   app.Image.ID,
		// TODO this is currently unknown and empty
		StageID: app.Stage.ID,
	})

	if err != nil {
		return errors.Wrap(err, "failed to render kube resource definition")
	}

	return nil
}

func renderService(filePath, org string, appName string) error {
	serviceTmpl, err := template.New("service").Parse(`
apiVersion: v1
kind: Service
metadata:
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
  labels:
    app.kubernetes.io/component: application
    app.kubernetes.io/managed-by: epinio
    app.kubernetes.io/name: {{ .AppName }}
    app.kubernetes.io/part-of: {{ .Org }}
    kubernetes.io/ingress.class: traefik
  name: {{ .AppName }}
  namespace: {{ .Org }}
spec:
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app.kubernetes.io/component: "application"
    app.kubernetes.io/name: "{{ .AppName }}"
  type: ClusterIP
  `)
	if err != nil {
		return errors.Wrap(err, "failed to parse service template for app")
	}

	serviceFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to create file for application Service definition")
	}
	defer func() { err = serviceFile.Close() }()

	err = serviceTmpl.Execute(serviceFile, struct {
		AppName string
		Org     string
	}{
		AppName: appName,
		Org:     org,
	})
	if err != nil {
		return errors.Wrap(err, "failed to render application Service definition")
	}

	return nil
}

func renderIngress(filePath, org, appName, route string) error {
	ingressTmpl, err := template.New("ingress").Parse(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
  labels:
    app.kubernetes.io/component: application
    app.kubernetes.io/managed-by: epinio
    app.kubernetes.io/name: {{ .AppName }}
    app.kubernetes.io/part-of: {{ .Org }}
    kubernetes.io/ingress.class: traefik
  name: {{ .AppName }}
  namespace: {{ .Org }}
spec:
  rules:
  - host: {{ .Route }}
    http:
      paths:
      - backend:
          service:
            name: {{ .AppName }}
            port:
              number: 8080
        path: /
        pathType: ImplementationSpecific
  tls:
  - hosts:
    - {{ .Route }}
    secretName: {{ .AppName }}-tls
  `)
	if err != nil {
		return errors.Wrap(err, "failed to parse ingress template for app")
	}

	ingressFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to create file for application Ingress definition")
	}
	defer func() { _ = ingressFile.Close() }()

	err = ingressTmpl.Execute(ingressFile, struct {
		AppName string
		Org     string
		Route   string
	}{
		AppName: appName,
		Org:     org,
		Route:   route,
	})
	if err != nil {
		return errors.Wrap(err, "failed to render application Ingress definition")
	}

	return nil
}
