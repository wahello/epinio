package acceptance_test

import (
	"fmt"

	"github.com/epinio/epinio/acceptance/helpers/catalog"
	"github.com/epinio/epinio/pkg/api/core/v1/models"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Services", func() {

	Describe("Catalog", func() {
		It("lists the standard catalog", func() {
			out, err := env.Epinio("", "service", "catalog")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("Getting catalog"))
			Expect(out).To(MatchRegexp("mysql-dev"))
			Expect(out).To(MatchRegexp("postgresql-dev"))
			Expect(out).To(MatchRegexp("rabbitmq-dev"))
			Expect(out).To(MatchRegexp("redis-dev"))
		})

		It("lists the catalog details", func() {
			out, err := env.Epinio("", "service", "catalog", "redis-dev")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("Name.*\\|.*redis-dev"))
			Expect(out).To(MatchRegexp("Short Description.*\\|.*A Redis service"))
			Expect(out).To(MatchRegexp("Bitnami Redis instance"))
			Expect(out).To(MatchRegexp("Version.*\\|.*6.2.7"))
		})

		When("Adding a catalog entry", func() {
			var catalogService models.CatalogService
			var serviceName string

			BeforeEach(func() {
				serviceName = catalog.NewCatalogServiceName()

				catalogService = models.CatalogService{
					Name:      serviceName,
					HelmChart: "nginx",
					HelmRepo: models.HelmRepo{
						Name: "",
						URL:  "https://charts.bitnami.com/bitnami",
					},
					Values: "{'service': {'type': 'ClusterIP'}}",
				}

				catalog.CreateCatalogService(catalogService)
			})

			AfterEach(func() {
				catalog.DeleteCatalogService(serviceName)
			})

			It("lists the extended catalog", func() {
				out, err := env.Epinio("", "service", "catalog")
				Expect(err).ToNot(HaveOccurred(), out)

				Expect(out).To(MatchRegexp("Getting catalog"))
				Expect(out).To(MatchRegexp(serviceName))
			})

			It("lists the extended catalog details", func() {
				out, err := env.Epinio("", "service", "catalog", serviceName)
				Expect(err).ToNot(HaveOccurred(), out)

				Expect(out).To(MatchRegexp(fmt.Sprintf("Name.*\\|.*%s", serviceName)))
			})
		})
	})

	Describe("delete services", func() {
		var namespace, service string

		BeforeEach(func() {
			namespace = catalog.NewNamespaceName()
			env.SetupAndTargetNamespace(namespace)

			service = catalog.NewServiceName()

			out, err := env.Epinio("", "service", "create", "mysql-dev", service)
			Expect(err).ToNot(HaveOccurred(), out)

			out, err = env.Epinio("", "service", "show", service)
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp(fmt.Sprintf("Name.*\\|.*%s", service)))
		})

		AfterEach(func() {
			env.DeleteNamespace(namespace)
		})

		It("deletes a service", func() {
			out, err := env.Epinio("", "service", "delete", service)
			Expect(err).ToNot(HaveOccurred(), out)

			Eventually(func() string {
				out, _ := env.Epinio("", "service", "delete", service)
				return out
			}, "1m", "5s").Should(MatchRegexp("service not found"))
		})
	})

	Describe("unbind services", func() {
		var namespace, service, app, containerImageURL string

		BeforeEach(func() {
			containerImageURL = "splatform/sample-app"

			namespace = catalog.NewNamespaceName()
			env.SetupAndTargetNamespace(namespace)

			service = catalog.NewServiceName()

			out, err := env.Epinio("", "service", "create", "mysql-dev", service)
			Expect(err).ToNot(HaveOccurred(), out)

			app = catalog.NewAppName()
			env.MakeContainerImageApp(app, 1, containerImageURL)

			Eventually(func() string {
				out, _ := env.Epinio("", "service", "show", service)
				return out
			}, "2m", "5s").Should(MatchRegexp("Status.*\\|.*deployed"))

			out, err = env.Epinio("", "service", "bind", service, app)
			Expect(err).ToNot(HaveOccurred(), out)
		})

		AfterEach(func() {
			env.DeleteNamespace(namespace)
		})

		It("unbinds the service", func() {
			appShowOut, err := env.Epinio("", "app", "show", app)
			Expect(err).ToNot(HaveOccurred())
			matchString := fmt.Sprintf("Bound Configurations.*%s", service)
			Expect(appShowOut).To(MatchRegexp(matchString))

			out, err := env.Epinio("", "service", "unbind", service, app)
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).ToNot(MatchRegexp("Available Commands:")) // Command should exist

			appShowOut, err = env.Epinio("", "app", "show", app)
			Expect(err).ToNot(HaveOccurred())
			matchString = fmt.Sprintf("Bound Configurations.*%s", service)
			Expect(appShowOut).ToNot(MatchRegexp(matchString))
		})
	})
})
