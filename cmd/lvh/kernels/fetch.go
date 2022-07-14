package kernels

import (
	"context"

	"github.com/cilium/little-vm-helper/pkg/kernels"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func fetchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <kernel>",
		Short: "fetch kernel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logrus.New()
			kname := args[0]
			return kernels.FetchKernel(context.Background(), log, dirName, kname)
		},
	}
}
