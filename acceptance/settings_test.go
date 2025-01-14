package acceptance_test

import (
	"fmt"

	"github.com/epinio/epinio/acceptance/helpers/catalog"
	"github.com/epinio/epinio/acceptance/helpers/proc"
	"github.com/epinio/epinio/acceptance/testenv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Settings", func() {
	var tmpSettingsPath string

	BeforeEach(func() {
		tmpSettingsPath = catalog.NewTmpName("tmpEpinio") + `.yaml`
	})

	AfterEach(func() {
		// Remove transient settings
		out, err := proc.Run("", false, "rm", "-f", tmpSettingsPath)
		Expect(err).ToNot(HaveOccurred(), out)
	})

	Describe("Ensemble", func() {
		It("fails for a bogus sub command", func() {
			out, err := env.Epinio("", "settings", "bogus", "...")
			Expect(err).To(HaveOccurred())
			Expect(out).To(MatchRegexp(`Unknown method "bogus"`))
		})
	})

	Describe("Colors", func() {
		It("changes the settings when disabling colors", func() {
			settings, err := env.Epinio("", "settings", "colors", "0", "--settings-file", tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(settings).To(MatchRegexp(`Colors: false`))

			settings, err = env.Epinio("", "settings", "show", "--settings-file", tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(settings).To(MatchRegexp(`Colorized Output.*\|.*false`))
		})

		It("changes the settings when enabling colors", func() {
			settings, err := env.Epinio("", "settings", "colors", "1", "--settings-file", tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(settings).To(MatchRegexp(`Colors: true`))

			settings, err = env.Epinio("", "settings", "show", "--settings-file", tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(settings).To(MatchRegexp(`Colorized Output.*\|.*true`))
		})
	})

	Describe("Show", func() {
		It("shows the settings", func() {
			settings, err := env.Epinio("", "settings", "show")
			Expect(err).ToNot(HaveOccurred())
			Expect(settings).To(MatchRegexp(`Colorized Output.*\|`))  // Exact state not relevant
			Expect(settings).To(MatchRegexp(`Current Namespace.*\|`)) // Exact name of namespace is not relevant, and varies
			Expect(settings).To(MatchRegexp(`Certificates.*\|.*Present`))
			Expect(settings).To(MatchRegexp(fmt.Sprintf(`API User Name.*\|.*%s`, env.EpinioUser)))
			Expect(settings).To(MatchRegexp(fmt.Sprintf(`API Password.*\|.*%s`, env.EpinioPassword)))
			Expect(settings).To(MatchRegexp(`API Url.*\| https://epinio.*`))
			Expect(settings).To(MatchRegexp(`WSS Url.*\| wss://epinio.*`))
		})
	})

	Describe("Update", func() {
		oldSettingsPath := testenv.EpinioYAML()

		It("regenerates certs and credentials", func() {
			// Get back the certs and credentials
			// Note that `namespace`, as a purely local setting, is not restored

			out, err := env.Epinio("", "settings", "update", "--settings-file", tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(MatchRegexp(`Updating the stored credentials`))

			oldSettings, err := env.GetSettingsFrom(oldSettingsPath)
			Expect(err).ToNot(HaveOccurred())

			newSettings, err := env.GetSettingsFrom(tmpSettingsPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(newSettings.User).To(Equal(oldSettings.User))
			Expect(newSettings.Password).To(Equal(oldSettings.Password))
			Expect(newSettings.API).To(Equal(oldSettings.API))
			Expect(newSettings.WSS).To(Equal(oldSettings.WSS))
			Expect(newSettings.Certs).To(Equal(oldSettings.Certs))
		})
	})
})
