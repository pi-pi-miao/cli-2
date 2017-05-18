package client

import (
	"encoding/json"
	"fmt"

	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	registrytypes "github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/opencontainers/go-digest"
	"golang.org/x/net/context"
)

// ServiceCreate creates a new Service.
func (cli *Client) ServiceCreate(ctx context.Context, service swarm.ServiceSpec, options types.ServiceCreateOptions) (types.ServiceCreateResponse, error) {
	var distErr error

	headers := map[string][]string{
		"version": {cli.version},
	}

	if options.EncodedRegistryAuth != "" {
		headers["X-Registry-Auth"] = []string{options.EncodedRegistryAuth}
	}

	// Contact the registry to retrieve digest and platform information
	if options.QueryRegistry {
		distributionInspect, err := cli.DistributionInspect(ctx, service.TaskTemplate.ContainerSpec.Image, options.EncodedRegistryAuth)
		distErr = err
		if err == nil {
			// now pin by digest if the image doesn't already contain a digest
			img := imageWithDigestString(service.TaskTemplate.ContainerSpec.Image, distributionInspect.Descriptor.Digest)
			if img != "" {
				service.TaskTemplate.ContainerSpec.Image = img
			}
			// add platforms that are compatible with the service
			service.TaskTemplate.Placement = updateServicePlatforms(service.TaskTemplate.Placement, distributionInspect)
		}
	}
	var response types.ServiceCreateResponse
	resp, err := cli.post(ctx, "/services/create", nil, service, headers)
	if err != nil {
		return response, err
	}

	err = json.NewDecoder(resp.body).Decode(&response)

	if distErr != nil {
		response.Warnings = append(response.Warnings, digestWarning(service.TaskTemplate.ContainerSpec.Image))
	}

	ensureReaderClosed(resp)
	return response, err
}

// imageWithDigestString takes an image string and a digest, and updates
// the image string if it didn't originally contain a digest. It assumes
// that the image string is not an image ID
func imageWithDigestString(image string, dgst digest.Digest) string {
	isCanonical := false
	ref, err := reference.ParseAnyReference(image)
	if err == nil {
		_, isCanonical = ref.(reference.Canonical)

		if !isCanonical {
			namedRef, _ := ref.(reference.Named)
			img, err := reference.WithDigest(namedRef, dgst)
			if err == nil {
				return img.String()
			}
		}
	}
	return ""
}

// updateServicePlatforms updates the Platforms in swarm.Placement to list
// all compatible platforms for the service, as found in distributionInspect
// and returns a pointer to the new or updated swarm.Placement struct
func updateServicePlatforms(placement *swarm.Placement, distributionInspect registrytypes.DistributionInspect) *swarm.Placement {
	if placement == nil {
		placement = &swarm.Placement{}
	}
	for _, p := range distributionInspect.Platforms {
		placement.Platforms = append(placement.Platforms, swarm.Platform{
			Architecture: p.Architecture,
			OS:           p.OS,
		})
	}
	return placement
}

// digestWarning constructs a formatted warning string using the
// image name that could not be pinned by digest. The formatting
// is hardcoded, but could me made smarter in the future
func digestWarning(image string) string {
	return fmt.Sprintf("image %s could not be accessed on a registry to record\nits digest. Each node will access %s independently,\npossibly leading to different nodes running different\nversions of the image.\n", image, image)
}
