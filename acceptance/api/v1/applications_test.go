package v1_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/epinio/epinio/acceptance/helpers/catalog"
	"github.com/epinio/epinio/acceptance/testenv"
	"github.com/epinio/epinio/deployments"
	"github.com/epinio/epinio/helpers"
	"github.com/epinio/epinio/helpers/randstr"
	v1 "github.com/epinio/epinio/internal/api/v1"
	"github.com/epinio/epinio/internal/domain"
	"github.com/epinio/epinio/internal/routes"
	apierrors "github.com/epinio/epinio/pkg/api/core/v1/errors"
	"github.com/epinio/epinio/pkg/api/core/v1/models"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Apps API Application Endpoints", func() {
	var (
		namespace string
	)
	containerImageURL := "splatform/sample-app"

	uploadRequest := func(url, path string) (*http.Request, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open tarball")
		}
		defer file.Close()

		// create multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", filepath.Base(file.Name()))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create multiform part")
		}

		_, err = io.Copy(part, file)
		if err != nil {
			return nil, errors.Wrap(err, "failed to write to multiform part")
		}

		err = writer.Close()
		if err != nil {
			return nil, errors.Wrap(err, "failed to close multiform")
		}

		// make the request
		request, err := http.NewRequest("POST", url, body)
		request.SetBasicAuth(env.EpinioUser, env.EpinioPassword)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build request")
		}
		request.Header.Add("Content-Type", writer.FormDataContentType())

		return request, nil
	}

	appFromAPI := func(namespace, app string) models.App {
		response, err := env.Curl("GET",
			fmt.Sprintf("%s%s/namespaces/%s/applications/%s",
				serverURL, v1.Root, namespace, app),
			strings.NewReader(""))

		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, response).ToNot(BeNil())
		defer response.Body.Close()
		ExpectWithOffset(1, response.StatusCode).To(Equal(http.StatusOK))
		bodyBytes, err := ioutil.ReadAll(response.Body)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		var responseApp models.App
		err = json.Unmarshal(bodyBytes, &responseApp)
		ExpectWithOffset(1, err).ToNot(HaveOccurred(), string(bodyBytes))
		ExpectWithOffset(1, responseApp.Meta.Name).To(Equal(app))
		ExpectWithOffset(1, responseApp.Meta.Namespace).To(Equal(namespace))

		return responseApp
	}

	updateAppInstances := func(namespace string, app string, instances int32) (int, []byte) {
		desired := instances
		data, err := json.Marshal(models.ApplicationUpdateRequest{
			Instances: &desired,
		})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		response, err := env.Curl("PATCH",
			fmt.Sprintf("%s%s/namespaces/%s/applications/%s",
				serverURL, v1.Root, namespace, app),
			strings.NewReader(string(data)))
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, response).ToNot(BeNil())

		defer response.Body.Close()
		bodyBytes, err := ioutil.ReadAll(response.Body)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		return response.StatusCode, bodyBytes
	}

	updateAppInstancesNAN := func(namespace string, app string) (int, []byte) {
		desired := int32(314)
		data, err := json.Marshal(models.ApplicationUpdateRequest{
			Instances: &desired,
		})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		// Hack to make the Instances value non-number
		data = []byte(strings.Replace(string(data), "314", `"thisisnotanumber"`, 1))

		response, err := env.Curl("PATCH",
			fmt.Sprintf("%s%s/namespaces/%s/applications/%s",
				serverURL, v1.Root, namespace, app),
			strings.NewReader(string(data)))
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, response).ToNot(BeNil())

		defer response.Body.Close()
		bodyBytes, err := ioutil.ReadAll(response.Body)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		return response.StatusCode, bodyBytes
	}

	createApplication := func(name string, namespace string, routes []string) (*http.Response, error) {
		request := models.ApplicationCreateRequest{
			Name: name,
			Configuration: models.ApplicationUpdateRequest{
				Routes: routes,
			},
		}
		b, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		body := string(b)

		url := serverURL + v1.Root + "/" + v1.Routes.Path("AppCreate", namespace)
		return env.Curl("POST", url, strings.NewReader(body))
	}

	waitForPipeline := func(stageID string) {
		Eventually(func() string {
			out, err := helpers.Kubectl("get", "pipelinerun",
				"--namespace", deployments.TektonStagingNamespace,
				stageID,
				"-o", "jsonpath={.status.conditions[0].status}")
			Expect(err).NotTo(HaveOccurred())
			return out
		}, "5m").Should(Equal("True"))
	}

	uploadApplication := func(appName string) *models.UploadResponse {
		uploadURL := serverURL + v1.Root + "/" + v1.Routes.Path("AppUpload", namespace, appName)
		uploadPath := testenv.TestAssetPath("sample-app.tar")
		uploadRequest, err := uploadRequest(uploadURL, uploadPath)
		Expect(err).ToNot(HaveOccurred())
		resp, err := env.Client().Do(uploadRequest)
		Expect(err).ToNot(HaveOccurred())
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		respObj := &models.UploadResponse{}
		err = json.Unmarshal(bodyBytes, &respObj)
		Expect(err).ToNot(HaveOccurred())

		return respObj
	}

	// returns all the objects currently stored on the S3 storage
	listS3Blobs := func() []string {
		out, err := helpers.Kubectl("get", "secret",
			"-n", "minio-epinio",
			"tenant-creds", "-o", "jsonpath={.data.accesskey}")
		Expect(err).ToNot(HaveOccurred(), out)
		accessKey, err := base64.StdEncoding.DecodeString(string(out))
		Expect(err).ToNot(HaveOccurred(), string(out))

		out, err = helpers.Kubectl("get", "secret",
			"-n", "minio-epinio",
			"tenant-creds", "-o", "jsonpath={.data.secretkey}")
		Expect(err).ToNot(HaveOccurred(), out)
		secretKey, err := base64.StdEncoding.DecodeString(string(out))
		Expect(err).ToNot(HaveOccurred(), string(out))

		rand, err := randstr.Hex16()
		Expect(err).ToNot(HaveOccurred(), out)
		// Setup "mc" to talk to our minio endpoint (the "mc alias" command)
		// and list all objects in the bucket (the "mc --quiet ls" command)
		out, err = helpers.Kubectl("run", "-it",
			"--restart=Never", "miniocli"+rand, "--rm",
			"--image=minio/mc", "--command", "--",
			"/bin/bash", "-c",
			fmt.Sprintf("mc alias set minio http://minio.minio-epinio.svc.cluster.local %s %s 2>&1 > /dev/null && mc --quiet ls minio/epinio", string(accessKey), string(secretKey)))
		Expect(err).ToNot(HaveOccurred(), out)

		return strings.Split(string(out), "\n")
	}

	stageApplication := func(appName, namespace string, uploadResponse *models.UploadResponse) *models.StageResponse {
		request := models.StageRequest{
			App: models.AppRef{
				Name:      appName,
				Namespace: namespace,
			},
			BlobUID:      uploadResponse.BlobUID,
			BuilderImage: "paketobuildpacks/builder:full",
		}
		b, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		body := string(b)

		url := serverURL + v1.Root + "/" + v1.Routes.Path("AppStage", namespace, appName)
		response, err := env.Curl("POST", url, strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		b, err = ioutil.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())

		stage := &models.StageResponse{}
		err = json.Unmarshal(b, stage)
		Expect(err).NotTo(HaveOccurred())

		waitForPipeline(stage.Stage.ID)

		return stage
	}

	BeforeEach(func() {
		namespace = catalog.NewNamespaceName()
		env.SetupAndTargetNamespace(namespace)

		// Wait for server to be up and running
		Eventually(func() error {
			_, err := env.Curl("GET", serverURL+v1.Root+"/info", strings.NewReader(""))
			return err
		}, "1m").ShouldNot(HaveOccurred())
	})
	AfterEach(func() {
		env.DeleteNamespace(namespace)
	})

	Context("Apps", func() {
		Describe("POST /namespaces/:namespace/applications/:app/import-git", func() {
			It("imports the git repo in the blob store", func() {
				app := catalog.NewAppName()
				gitURL := "https://github.com/epinio/example-wordpress"
				data := url.Values{}
				data.Set("giturl", gitURL)
				data.Set("gitrev", "main")

				url := serverURL + v1.Root + "/" + v1.Routes.Path("AppImportGit", namespace, app)
				request, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
				Expect(err).ToNot(HaveOccurred())
				request.SetBasicAuth(env.EpinioUser, env.EpinioPassword)
				request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
				request.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

				response, err := env.Client().Do(request)
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred(), string(bodyBytes))
				Expect(response.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

				var importResponse models.ImportGitResponse
				err = json.Unmarshal(bodyBytes, &importResponse)
				Expect(err).ToNot(HaveOccurred())
				Expect(importResponse.BlobUID).ToNot(BeEmpty())
				Expect(importResponse.BlobUID).To(MatchRegexp(".+-.+-.+-.+-.+"))
			})
		})

		Describe("PATCH /namespaces/:namespace/applications/:app", func() {
			When("instances is valid integer", func() {
				It("updates an application with the desired number of instances", func() {
					app := catalog.NewAppName()
					env.MakeContainerImageApp(app, 1, containerImageURL)
					defer env.DeleteApp(app)

					appObj := appFromAPI(namespace, app)
					Expect(appObj.Workload.Status).To(Equal("1/1"))

					status, _ := updateAppInstances(namespace, app, 3)
					Expect(status).To(Equal(http.StatusOK))

					Eventually(func() string {
						return appFromAPI(namespace, app).Workload.Status
					}, "1m").Should(Equal("3/3"))
				})
			})

			When("instances is invalid", func() {
				It("returns BadRequest when instances is a negative number", func() {
					app := catalog.NewAppName()
					env.MakeContainerImageApp(app, 1, containerImageURL)
					defer env.DeleteApp(app)
					Expect(appFromAPI(namespace, app).Workload.Status).To(Equal("1/1"))

					status, updateResponseBody := updateAppInstances(namespace, app, -3)
					Expect(status).To(Equal(http.StatusBadRequest))

					var errorResponse apierrors.ErrorResponse
					err := json.Unmarshal(updateResponseBody, &errorResponse)
					Expect(err).ToNot(HaveOccurred())
					Expect(errorResponse.Errors[0].Status).To(Equal(http.StatusBadRequest))
					Expect(errorResponse.Errors[0].Title).To(Equal("instances param should be integer equal or greater than zero"))
				})

				It("returns BadRequest when instances is not a number", func() {
					// The bad request does not even reach deeper validation, as it fails to
					// convert into the expected structure.

					app := catalog.NewAppName()
					env.MakeContainerImageApp(app, 1, containerImageURL)
					defer env.DeleteApp(app)
					Expect(appFromAPI(namespace, app).Workload.Status).To(Equal("1/1"))

					status, updateResponseBody := updateAppInstancesNAN(namespace, app)
					Expect(status).To(Equal(http.StatusBadRequest))

					var errorResponse apierrors.ErrorResponse
					err := json.Unmarshal(updateResponseBody, &errorResponse)
					Expect(err).ToNot(HaveOccurred())
					Expect(errorResponse.Errors[0].Status).To(Equal(http.StatusBadRequest))
					Expect(errorResponse.Errors[0].Title).To(Equal("json: cannot unmarshal string into Go struct field ApplicationUpdateRequest.instances of type int32"))
				})
			})
			When("routes have changed", func() {
				// removes empty strings from the given slice
				deleteEmpty := func(elements []string) []string {
					var result []string
					for _, e := range elements {
						if e != "" {
							result = append(result, e)
						}
					}
					return result
				}

				checkCertificateDNSNames := func(appName, namespaceName string, routes ...string) {
					Eventually(func() int {
						out, err := helpers.Kubectl("get", "certificates",
							"-n", namespaceName,
							"--selector", "app.kubernetes.io/name="+appName,
							"-o", "jsonpath={.items[*].spec.dnsNames[*]}")
						Expect(err).ToNot(HaveOccurred(), out)
						return len(deleteEmpty(strings.Split(out, " ")))
					}, "20s", "1s").Should(Equal(len(routes)))

					out, err := helpers.Kubectl("get", "certificates",
						"-n", namespaceName,
						"--selector", "app.kubernetes.io/name="+appName,
						"-o", "jsonpath={.items[*].spec.dnsNames[*]}")
					Expect(err).ToNot(HaveOccurred(), out)
					certDomains := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))
					Expect(certDomains).To(ContainElements(routes))
					Expect(len(certDomains)).To(Equal(len(routes)))
				}

				checkIngresses := func(appName, namespaceName string, routesStr ...string) {
					routeObjects := []routes.Route{}
					for _, route := range routesStr {
						routeObjects = append(routeObjects, routes.FromString(route))
					}

					Eventually(func() int {
						out, err := helpers.Kubectl("get", "ingresses",
							"-n", namespaceName,
							"--selector", "app.kubernetes.io/name="+appName,
							"-o", "jsonpath={.items[*].spec.rules[*].host}")
						Expect(err).ToNot(HaveOccurred(), out)
						return len(deleteEmpty(strings.Split(out, " ")))
					}, "20s", "1s").Should(Equal(len(routeObjects)))

					out, err := helpers.Kubectl("get", "ingresses",
						"-n", namespaceName,
						"--selector", "app.kubernetes.io/name="+appName,
						"-o", "jsonpath={range .items[*]}{@.spec.rules[0].host}{@.spec.rules[0].http.paths[0].path} ")
					Expect(err).ToNot(HaveOccurred(), out)
					ingressRoutes := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))
					trimmedRoutes := []string{}
					for _, ir := range ingressRoutes {
						trimmedRoutes = append(trimmedRoutes, strings.TrimSuffix(ir, "/"))
					}
					Expect(trimmedRoutes).To(ContainElements(routesStr))
					Expect(len(trimmedRoutes)).To(Equal(len(routesStr)))
				}

				// Checks if every secret referenced in a certificate of the given app,
				// has a corresponding secret. routes are used to wait until all
				// certificates are created.
				checkSecretsForCerts := func(appName, namespaceName string, routes ...string) {
					Eventually(func() int {
						out, err := helpers.Kubectl("get", "certificates",
							"-n", namespaceName,
							"--selector", "app.kubernetes.io/name="+appName,
							"-o", "jsonpath={.items[*].spec.secretName}")
						Expect(err).ToNot(HaveOccurred(), out)
						certSecrets := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))
						return len(certSecrets)
					}, "20s", "1s").Should(Equal(len(routes)))

					out, err := helpers.Kubectl("get", "certificates",
						"-n", namespaceName,
						"--selector", "app.kubernetes.io/name="+appName,
						"-o", "jsonpath={.items[*].spec.secretName}")
					Expect(err).ToNot(HaveOccurred(), out)
					certSecrets := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))

					Eventually(func() []string {
						out, err = helpers.Kubectl("get", "secrets", "-n", namespaceName, "-o", "jsonpath={.items[*].metadata.name}")
						Expect(err).ToNot(HaveOccurred(), out)
						existingSecrets := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))
						return existingSecrets
					}, "60s", "1s").Should(ContainElements(certSecrets))
				}

				checkRoutesOnApp := func(appName, namespaceName string, routes ...string) {
					out, err := helpers.Kubectl("get", "apps", "-n", namespaceName, appName, "-o", "jsonpath={.spec.routes[*]}")
					Expect(err).ToNot(HaveOccurred(), out)
					appRoutes := deleteEmpty(strings.Split(strings.TrimSpace(out), " "))
					Expect(appRoutes).To(Equal(routes))
				}

				It("synchronizes the ingresses of the application with the new routes list", func() {
					app := catalog.NewAppName()
					env.MakeContainerImageApp(app, 1, containerImageURL)
					defer env.DeleteApp(app)

					mainDomain, err := domain.MainDomain(context.Background())
					Expect(err).ToNot(HaveOccurred())

					checkRoutesOnApp(app, namespace, fmt.Sprintf("%s.%s", app, mainDomain))
					checkIngresses(app, namespace, fmt.Sprintf("%s.%s", app, mainDomain))
					checkCertificateDNSNames(app, namespace, fmt.Sprintf("%s.%s", app, mainDomain))
					checkSecretsForCerts(app, namespace, fmt.Sprintf("%s.%s", app, mainDomain))

					appObj := appFromAPI(namespace, app)
					Expect(appObj.Workload.Status).To(Equal("1/1"))

					newRoutes := []string{"domain1.org", "domain2.org"}
					data, err := json.Marshal(models.ApplicationUpdateRequest{
						Routes: newRoutes,
					})
					Expect(err).ToNot(HaveOccurred())

					response, err := env.Curl("PATCH",
						fmt.Sprintf("%s%s/namespaces/%s/applications/%s",
							serverURL, v1.Root, namespace, app),
						strings.NewReader(string(data)))
					Expect(err).ToNot(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK))

					checkRoutesOnApp(app, namespace, newRoutes...)
					checkIngresses(app, namespace, newRoutes...)
					checkCertificateDNSNames(app, namespace, newRoutes...)
					checkSecretsForCerts(app, namespace, newRoutes...)
				})
			})
		})

		Describe("GET /api/v1/namespaces/:namespaces/applications", func() {
			It("lists all applications belonging to the namespace", func() {
				app1 := catalog.NewAppName()
				env.MakeContainerImageApp(app1, 1, containerImageURL)
				defer env.DeleteApp(app1)
				app2 := catalog.NewAppName()
				env.MakeContainerImageApp(app2, 1, containerImageURL)
				defer env.DeleteApp(app2)

				response, err := env.Curl("GET", fmt.Sprintf("%s%s/namespaces/%s/applications",
					serverURL, v1.Root, namespace), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())
				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

				var apps models.AppList
				err = json.Unmarshal(bodyBytes, &apps)
				Expect(err).ToNot(HaveOccurred())

				appNames := []string{apps[0].Meta.Name, apps[1].Meta.Name}
				Expect(appNames).To(ContainElements(app1, app2))

				namespaceNames := []string{apps[0].Meta.Namespace, apps[1].Meta.Namespace}
				Expect(namespaceNames).To(ContainElements(namespace, namespace))

				// Applications are deployed. Must have workload.
				statuses := []string{apps[0].Workload.Status, apps[1].Workload.Status}
				Expect(statuses).To(ContainElements("1/1", "1/1"))
			})

			It("returns a 404 when the namespace does not exist", func() {
				response, err := env.Curl("GET", fmt.Sprintf("%s%s/namespaces/idontexist/applications",
					serverURL, v1.Root), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound), string(bodyBytes))
			})
		})

		Describe("GET /api/v1/namespaces/:namespace/applications/:app", func() {
			It("lists the application data", func() {
				app := catalog.NewAppName()
				env.MakeContainerImageApp(app, 1, containerImageURL)
				defer env.DeleteApp(app)

				appObj := appFromAPI(namespace, app)
				Expect(appObj.Workload.Status).To(Equal("1/1"))
				createdAt, err := time.Parse(time.RFC3339, appObj.Workload.CreatedAt)
				Expect(err).ToNot(HaveOccurred())
				Expect(createdAt.Unix()).To(BeNumerically("<", time.Now().Unix()))

				Expect(appObj.Workload.Restarts).To(BeNumerically("==", 0))

				Expect(appObj.Workload.DesiredReplicas).To(BeNumerically("==", 1))
				Expect(appObj.Workload.ReadyReplicas).To(BeNumerically("==", 1))

				out, err := helpers.Kubectl("get", "pods",
					fmt.Sprintf("--selector=app.kubernetes.io/name=%s", app),
					"--namespace", namespace, "--output", "name")
				Expect(err).ToNot(HaveOccurred())
				podNames := strings.Split(string(out), "\n")

				// Run `yes > /dev/null &` and expect at least 1000 millicpus
				// https://winaero.com/how-to-create-100-cpu-load-in-linux/
				out, err = helpers.Kubectl("exec",
					"--namespace", namespace, podNames[0], "--container", app,
					"--", "bin/sh", "-c", "yes > /dev/null 2> /dev/null &")
				Expect(err).ToNot(HaveOccurred(), out)
				Eventually(func() int64 {
					appObj := appFromAPI(namespace, app)
					return appObj.Workload.MilliCPUs
				}, "240s", "1s").Should(BeNumerically(">=", 900))
				// Kill the "yes" process to bring CPU down again
				out, err = helpers.Kubectl("exec",
					"--namespace", namespace, podNames[0], "--container", app,
					"--", "killall", "-9", "yes")
				Expect(err).ToNot(HaveOccurred(), out)

				// Increase memory for 3 minutes to check memory metric
				out, err = helpers.Kubectl("exec",
					"--namespace", namespace, podNames[0], "--container", app,
					"--", "bin/bash", "-c", "cat <( </dev/zero head -c 50m) <(sleep 180) | tail")
				Expect(err).ToNot(HaveOccurred(), out)
				Eventually(func() int64 {
					appObj := appFromAPI(namespace, app)
					return appObj.Workload.MemoryBytes
				}, "240s", "1s").Should(BeNumerically(">=", 0))

				// Kill a linkerd proxy container and see the count staying unchanged
				out, err = helpers.Kubectl("exec",
					"--namespace", namespace, podNames[0], "--container", "linkerd-proxy",
					"--", "bin/sh", "-c", "kill 1")
				Expect(err).ToNot(HaveOccurred(), out)

				Consistently(func() int32 {
					appObj := appFromAPI(namespace, app)
					return appObj.Workload.Restarts
				}, "5s", "1s").Should(BeNumerically("==", 0))

				// Kill an app container and see the count increasing
				out, err = helpers.Kubectl("exec",
					"--namespace", namespace, podNames[0], "--container", app,
					"--", "bin/sh", "-c", "kill 1")
				Expect(err).ToNot(HaveOccurred(), out)

				Eventually(func() int32 {
					appObj := appFromAPI(namespace, app)
					return appObj.Workload.Restarts
				}, "4s", "1s").Should(BeNumerically("==", 1))
			})

			It("returns a 404 when the namespace does not exist", func() {
				app := catalog.NewAppName()
				env.MakeContainerImageApp(app, 1, containerImageURL)
				defer env.DeleteApp(app)

				response, err := env.Curl("GET", fmt.Sprintf("%s%s/namespaces/idontexist/applications/%s",
					serverURL, v1.Root, app), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound), string(bodyBytes))
			})

			It("returns a 404 when the app does not exist", func() {
				response, err := env.Curl("GET", fmt.Sprintf("%s%s/namespaces/%s/applications/bogus",
					serverURL, v1.Root, namespace), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound), string(bodyBytes))
			})
		})

		Describe("DELETE /api/v1/namespaces/:namespace/applications/:app", func() {
			It("removes the application, unbinds bound services", func() {
				app1 := catalog.NewAppName()
				env.MakeContainerImageApp(app1, 1, containerImageURL)
				service := catalog.NewServiceName()
				env.MakeService(service)
				env.BindAppService(app1, service, namespace)
				defer env.CleanupService(service)

				response, err := env.Curl("DELETE", fmt.Sprintf("%s%s/namespaces/%s/applications/%s",
					serverURL, v1.Root, namespace, app1), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())
				defer response.Body.Close()
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())

				var resp map[string][]string
				err = json.Unmarshal(bodyBytes, &resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp).To(HaveLen(1))
				Expect(resp).To(HaveKey("unboundservices"))
				Expect(resp["unboundservices"]).To(ContainElement(service))
			})

			It("returns a 404 when the namespace does not exist", func() {
				app1 := catalog.NewAppName()
				env.MakeContainerImageApp(app1, 1, containerImageURL)
				defer env.DeleteApp(app1)

				response, err := env.Curl("DELETE", fmt.Sprintf("%s%s/namespaces/idontexist/applications/%s",
					serverURL, v1.Root, app1), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound), string(bodyBytes))
			})

			It("returns a 404 when the app does not exist", func() {
				response, err := env.Curl("DELETE", fmt.Sprintf("%s%s/namespaces/%s/applications/bogus",
					serverURL, v1.Root, namespace), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound), string(bodyBytes))
			})
		})

		Describe("GET /api/v1/applications", func() {
			var namespace1 string
			var namespace2 string
			var app1 string
			var app2 string

			BeforeEach(func() {
				namespace1 = catalog.NewNamespaceName()
				env.SetupAndTargetNamespace(namespace1)

				app1 = catalog.NewAppName()
				env.MakeContainerImageApp(app1, 1, containerImageURL)

				namespace2 = catalog.NewNamespaceName()
				env.SetupAndTargetNamespace(namespace2)

				app2 = catalog.NewAppName()
				env.MakeContainerImageApp(app2, 1, containerImageURL)
			})
			AfterEach(func() {
				env.TargetNamespace(namespace2)
				env.DeleteApp(app2)

				env.TargetNamespace(namespace1)
				env.DeleteApp(app1)

				env.DeleteNamespace(namespace1)
				env.DeleteNamespace(namespace2)
			})
			It("lists all applications belonging to all namespaces", func() {
				response, err := env.Curl("GET", fmt.Sprintf("%s%s/applications",
					serverURL, v1.Root), strings.NewReader(""))
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())

				defer response.Body.Close()
				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

				var apps models.AppList
				err = json.Unmarshal(bodyBytes, &apps)
				Expect(err).ToNot(HaveOccurred())

				// `apps` contains all apps. Not just the two we are looking for, from
				// the setup of this test. Everything which still exists from other
				// tests executing concurrently, or not cleaned by previous tests, or
				// the setup, or ... So, we cannot be sure that the two apps are in the
				// two first elements of the slice.

				var appRefs [][]string
				for _, a := range apps {
					appRefs = append(appRefs, []string{a.Meta.Name, a.Meta.Namespace})
				}
				Expect(appRefs).To(ContainElements(
					[]string{app1, namespace1},
					[]string{app2, namespace2}))
			})
		})
	})

	Context("Uploading", func() {

		var (
			url     string
			path    string
			request *http.Request
		)

		JustBeforeEach(func() {
			url = serverURL + v1.Root + "/" + v1.Routes.Path("AppUpload", namespace, "testapp")
			var err error
			request, err = uploadRequest(url, path)
			Expect(err).ToNot(HaveOccurred())
		})

		When("uploading a new dir", func() {
			BeforeEach(func() {
				path = testenv.TestAssetPath("sample-app.tar")
			})

			It("returns the app response", func() {
				resp, err := env.Client().Do(request)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp).ToNot(BeNil())
				defer resp.Body.Close()

				bodyBytes, err := ioutil.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

				r := &models.UploadResponse{}
				err = json.Unmarshal(bodyBytes, &r)
				Expect(err).ToNot(HaveOccurred())

				Expect(r.BlobUID).ToNot(BeEmpty())
			})
		})
	})

	Context("Deploying", func() {
		var (
			url     string
			body    string
			appName string
			request models.DeployRequest
		)

		BeforeEach(func() {
			namespace = catalog.NewNamespaceName()
			env.SetupAndTargetNamespace(namespace)
			appName = catalog.NewAppName()

			By("creating application resource first")
			_, err := createApplication(appName, namespace, []string{})
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			env.DeleteApp(appName)
		})

		Context("with staging", func() {
			When("staging an app with the blob of a different app", func() {
				var appName2 string
				var uploadResponse2 *models.UploadResponse

				BeforeEach(func() {
					appName2 = catalog.NewAppName()

					By("creating the other application resource first")
					_, err := createApplication(appName2, namespace, []string{})
					Expect(err).ToNot(HaveOccurred())

					By("uploading the code of the other")
					uploadResponse2 = uploadApplication(appName2)

					By("uploading the code of itself")
					_ = uploadApplication(appName)
				})

				AfterEach(func() {
					env.DeleteApp(appName2)
				})

				It("fails to stage", func() {
					// Inlined stageApplication() to check for the error.
					// Note how appName and uploadResponse2 are mixed.

					request := models.StageRequest{
						App: models.AppRef{
							Name:      appName, // App 1
							Namespace: namespace,
						},
						BlobUID:      uploadResponse2.BlobUID, // Code 2
						BuilderImage: "paketobuildpacks/builder:full",
					}
					b, err := json.Marshal(request)
					Expect(err).NotTo(HaveOccurred())
					body := string(b)

					url := serverURL + v1.Root + "/" + v1.Routes.Path("AppStage", namespace, appName)
					response, err := env.Curl("POST", url, strings.NewReader(body))
					Expect(err).NotTo(HaveOccurred())

					b, err = ioutil.ReadAll(response.Body)
					Expect(err).NotTo(HaveOccurred())

					Expect(response.StatusCode).To(Equal(http.StatusBadRequest), string(b))

					errResponse := &apierrors.ErrorResponse{}
					err = json.Unmarshal(b, errResponse)
					Expect(err).NotTo(HaveOccurred())

					Expect(errResponse.Errors).To(HaveLen(1))
					Expect(errResponse.Errors[0].Title).To(Equal("blob app mismatch"))
					Expect(errResponse.Errors[0].Details).To(Equal("expected: " + appName + ", found: " + appName2))
				})
			})

			When("staging the same app with a new blob", func() {
				It("cleans up old S3 objects", func() {
					By("uploading the code")
					uploadResponse := uploadApplication(appName)
					oldBlob := uploadResponse.BlobUID
					By("staging the application")
					_ = stageApplication(appName, namespace, uploadResponse)
					Eventually(listS3Blobs, "1m").Should(ContainElement(ContainSubstring(oldBlob)))

					By("uploading the code again")
					uploadResponse = uploadApplication(appName)
					newBlob := uploadResponse.BlobUID
					By("staging the application again")
					_ = stageApplication(appName, namespace, uploadResponse)

					Eventually(listS3Blobs, "2m").Should(ContainElement(ContainSubstring(newBlob)))
					Eventually(listS3Blobs, "2m").ShouldNot(ContainElement(ContainSubstring(oldBlob)))
				})
			})

			When("deploying a new app", func() {
				It("returns a success", func() {
					By("uploading the code")
					uploadResponse := uploadApplication(appName)

					By("staging the application")
					stageResponse := stageApplication(appName, namespace, uploadResponse)

					By("deploying the staged resource")
					request = models.DeployRequest{
						App: models.AppRef{
							Name:      appName,
							Namespace: namespace,
						},
						Stage: models.StageRef{
							ID: stageResponse.Stage.ID,
						},
						ImageURL: stageResponse.ImageURL,
						Origin: models.ApplicationOrigin{
							Kind: models.OriginPath,
							Path: testenv.TestAssetPath("sample-app.tar"),
						},
					}

					bodyBytes, err := json.Marshal(request)
					Expect(err).ToNot(HaveOccurred())
					body = string(bodyBytes)

					url = serverURL + v1.Root + "/" + v1.Routes.Path("AppDeploy", namespace, appName)

					response, err := env.Curl("POST", url, strings.NewReader(body))
					Expect(err).ToNot(HaveOccurred())
					Expect(response).ToNot(BeNil())
					defer response.Body.Close()

					bodyBytes, err = ioutil.ReadAll(response.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

					deploy := &models.DeployResponse{}
					err = json.Unmarshal(bodyBytes, deploy)
					Expect(err).NotTo(HaveOccurred())
					Expect(deploy.Routes[0]).To(MatchRegexp(appName + `.*\.omg\.howdoi\.website`))

					By("waiting for the deployment to complete")

					url = serverURL + v1.Root + "/" + v1.Routes.Path("AppRunning", namespace, appName)

					response, err = env.Curl("GET", url, strings.NewReader(body))
					Expect(err).ToNot(HaveOccurred())
					Expect(response).ToNot(BeNil())
					defer response.Body.Close()

					By("confirming at highlevel")
					// Highlevel check and confirmation
					Eventually(func() string {
						return appFromAPI(namespace, appName).Workload.Status
					}, "5m").Should(Equal("1/1"))
				})
			})
		})

		Context("with non-staging using custom container image", func() {
			BeforeEach(func() {
				request = models.DeployRequest{
					App: models.AppRef{
						Name:      appName,
						Namespace: namespace,
					},
					ImageURL: "splatform/sample-app",
					Origin: models.ApplicationOrigin{
						Kind:      models.OriginContainer,
						Container: "splatform/sample-app",
					},
				}

				url = serverURL + v1.Root + "/" + v1.Routes.Path("AppDeploy", namespace, appName)
			})

			When("deploying a new app", func() {
				BeforeEach(func() {
					bodyBytes, err := json.Marshal(request)
					Expect(err).ToNot(HaveOccurred())
					body = string(bodyBytes)
				})

				It("returns a success", func() {
					response, err := env.Curl("POST", url, strings.NewReader(body))
					Expect(err).ToNot(HaveOccurred())
					Expect(response).ToNot(BeNil())
					defer response.Body.Close()

					bodyBytes, err := ioutil.ReadAll(response.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusOK), string(bodyBytes))

					deploy := &models.DeployResponse{}
					err = json.Unmarshal(bodyBytes, deploy)
					Expect(err).NotTo(HaveOccurred())
					Expect(deploy.Routes[0]).To(MatchRegexp(appName + `.*\.omg\.howdoi\.website`))

					Eventually(func() string {
						return appFromAPI(namespace, appName).Workload.Status
					}, "5m").Should(Equal("1/1"))

					// Check if autoserviceaccounttoken is true
					labels := fmt.Sprintf("app.kubernetes.io/name=%s", appName)
					out, err := helpers.Kubectl("get", "pod",
						"--namespace", namespace,
						"-l", labels,
						"-o", "jsonpath={.items[*].spec.automountServiceAccountToken}")
					Expect(err).NotTo(HaveOccurred(), out)
					Expect(out).To(ContainSubstring("true"))
				})
			})

			When("deploying an app with custom routes", func() {
				var routes []string
				BeforeEach(func() {
					routes = append(routes, "appdomain.org", "appdomain2.org")
					out, err := helpers.Kubectl("patch", "apps", "--type", "json",
						"-n", namespace, appName, "--patch",
						fmt.Sprintf(`[{"op": "replace", "path": "/spec/routes", "value": [%q, %q]}]`, routes[0], routes[1]))
					Expect(err).NotTo(HaveOccurred(), out)
				})

				It("the app Ingress matches the specified route", func() {
					bodyBytes, err := json.Marshal(request)
					Expect(err).ToNot(HaveOccurred())
					body = string(bodyBytes)
					// call the deploy action. Deploy should respect the routes on the App CR.
					_, err = env.Curl("POST", url, strings.NewReader(body))
					Expect(err).ToNot(HaveOccurred())

					out, err := helpers.Kubectl("get", "ingress",
						"--namespace", namespace, "-o", "jsonpath={.items[*].spec.rules[0].host}")
					Expect(err).NotTo(HaveOccurred(), out)
					Expect(strings.Split(out, " ")).To(Equal(routes))
				})
			})
		})
	})

	Context("Logs", func() {
		Describe("GET /api/v1/namespaces/:namespaces/applications/:app/logs", func() {
			logLength := 0
			var (
				route string
				app   string
			)

			BeforeEach(func() {
				app = catalog.NewAppName()
				out := env.MakeApp(app, 1, true)
				route = testenv.AppRouteFromOutput(out)
				Expect(route).ToNot(BeEmpty())
			})

			AfterEach(func() {
				env.DeleteApp(app)
			})

			readLogs := func(namespace, app string) string {
				var urlArgs = []string{}
				urlArgs = append(urlArgs, fmt.Sprintf("follow=%t", false))
				wsURL := fmt.Sprintf("%s%s/%s?%s", websocketURL, v1.Root, v1.Routes.Path("AppLogs", namespace, app), strings.Join(urlArgs, "&"))
				wsConn := env.MakeWebSocketConnection(wsURL)

				By("read the logs")
				var logs string
				Eventually(func() bool {
					_, message, err := wsConn.ReadMessage()
					logLength++
					logs = fmt.Sprintf("%s %s", logs, string(message))
					return websocket.IsCloseError(err, websocket.CloseNormalClosure)
				}, 30*time.Second, 1*time.Second).Should(BeTrue())

				err := wsConn.Close()
				// With regular `ws` we could expect to not see any errors. With `wss`
				// however, with a tls layer in the mix, we can expect to see a `broken
				// pipe` issued. That is not a thing to act on, and is ignored.
				if err != nil && strings.Contains(err.Error(), "broken pipe") {
					return logs
				}
				Expect(err).ToNot(HaveOccurred())

				return logs
			}

			It("should send the logs", func() {
				logs := readLogs(namespace, app)

				By("checking if the logs are right")
				podNames := env.GetPodNames(app, namespace)
				for _, podName := range podNames {
					Expect(logs).To(ContainSubstring(podName))
				}
			})

			It("should follow logs", func() {
				existingLogs := readLogs(namespace, app)
				logLength := len(strings.Split(existingLogs, "\n"))

				var urlArgs = []string{}
				urlArgs = append(urlArgs, fmt.Sprintf("follow=%t", true))
				wsURL := fmt.Sprintf("%s%s/%s?%s", websocketURL, v1.Root, v1.Routes.Path("AppLogs", namespace, app), strings.Join(urlArgs, "&"))
				wsConn := env.MakeWebSocketConnection(wsURL)

				By("get to the end of logs")
				for i := 0; i < logLength-1; i++ {
					_, message, err := wsConn.ReadMessage()
					Expect(err).NotTo(HaveOccurred())
					Expect(message).NotTo(BeNil())
				}

				By("adding more logs")
				Eventually(func() int {
					resp, err := env.Curl("GET", route, strings.NewReader(""))
					Expect(err).ToNot(HaveOccurred())

					defer resp.Body.Close()

					bodyBytes, err := ioutil.ReadAll(resp.Body)
					Expect(err).ToNot(HaveOccurred(), resp)

					// reply must be from the phpinfo app
					if !strings.Contains(string(bodyBytes), "phpinfo()") {
						return 0
					}

					return resp.StatusCode
				}, 30*time.Second, 1*time.Second).Should(Equal(http.StatusOK))

				By("checking the latest log message")
				Eventually(func() string {
					_, message, err := wsConn.ReadMessage()
					Expect(err).NotTo(HaveOccurred())
					Expect(message).NotTo(BeNil())
					return string(message)
				}, "10s").Should(ContainSubstring("GET / HTTP/1.1"))

				err := wsConn.Close()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Creating", func() {
		var (
			appName string
		)

		BeforeEach(func() {
			namespace = catalog.NewNamespaceName()
			env.SetupAndTargetNamespace(namespace)
			appName = catalog.NewAppName()
		})

		AfterEach(func() {
			Eventually(func() string {
				out, err := env.Epinio("", "app", "delete", appName)
				if err != nil {
					return out
				}
				return ""
			}, "5m").Should(BeEmpty())
		})

		When("creating a new app", func() {
			It("creates the app resource", func() {
				response, err := createApplication(appName, namespace, []string{"mytestdomain.org"})
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil())
				defer response.Body.Close()

				bodyBytes, err := ioutil.ReadAll(response.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusCreated), string(bodyBytes))
				out, err := helpers.Kubectl("get", "apps", "-n", namespace, appName, "-o", "jsonpath={.spec.routes[*]}")
				Expect(err).ToNot(HaveOccurred(), out)
				routes := strings.Split(out, " ")
				Expect(len(routes)).To(Equal(1))
				Expect(routes[0]).To(Equal("mytestdomain.org"))
			})
		})
	})
})
