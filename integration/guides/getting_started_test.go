package guides_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"

	"github.com/werf/werf/pkg/testing/utils"
	utilsDocker "github.com/werf/werf/pkg/testing/utils/docker"
)

var _ = Describe("Getting started", func() {
	AfterEach(func() {
		utils.RunSucceedCommand(
			testDirPath,
			werfBinPath,
			"stages", "purge", "-s", ":local", "--force",
		)
	})

	It("application should be built, checked and published", func() {
		utils.RunSucceedCommand(
			".",
			"git",
			"clone", "https://github.com/dockersamples/linux_tweet_app.git", testDirPath,
		)

		utils.CopyIn(utils.FixturePath("getting_started"), testDirPath)

		utils.RunSucceedCommand(
			testDirPath,
			werfBinPath,
			"build", "-s", ":local",
		)

		containerName := fmt.Sprintf("getting_started_%s", utils.GetRandomString(10))

		utils.RunSucceedCommand(
			testDirPath,
			werfBinPath,
			"run", "-s", ":local", "--docker-options", fmt.Sprintf("-d -p :80 --name %s", containerName),
		)
		defer func() { utilsDocker.ContainerStopAndRemove(containerName) }()

		url := fmt.Sprintf("http://localhost:%s", utilsDocker.ContainerHostPort(containerName, "80/tcp"))
		waitTillHostReadyAndCheckResponseBody(
			url,
			utils.DefaultWaitTillHostReadyToRespondMaxAttempts,
			"Linux Tweet App!",
		)

		utils.RunSucceedCommand(
			testDirPath,
			werfBinPath,
			"publish", "-s", ":local", "-i", registryProjectRepository, "--tag-custom", "test",
		)
	})
})
