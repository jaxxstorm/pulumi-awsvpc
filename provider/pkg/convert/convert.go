package convert

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// IDOutputArraytoIDArrayOutput converts a slice of pulumi.IDOutput to the Pulumi type IDArrayOutput
func IDOutputSlicetoIDArrayOutput(as []pulumi.IDOutput) pulumi.IDArrayOutput {
	var outputs []interface{}
	for _, a := range as {
		outputs = append(outputs, a)
	}
	return pulumi.All(outputs...).ApplyT(func(vs []interface{}) []pulumi.ID {
		var results []pulumi.ID
		for _, v := range vs {
			results = append(results, v.(pulumi.ID))
		}
		return results
	}).(pulumi.IDArrayOutput)
}
