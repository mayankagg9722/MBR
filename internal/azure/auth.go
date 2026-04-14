// Package azure provides shared Azure SDK configuration helpers.
package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Cred is a type alias for the DefaultAzureCredential for convenience.
type Cred = azidentity.DefaultAzureCredential

// LoadCredential builds an Azure credential using the default credential chain
// (env vars → managed identity → Azure CLI → Azure Developer CLI).
// This mirrors the AWS LoadConfig pattern from the mbr project.
func LoadCredential() (*Cred, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("load Azure credential: %w", err)
	}
	return cred, nil
}
