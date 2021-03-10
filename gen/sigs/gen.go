// +build ignore

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kenshaw/hkp"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	keys := map[string][]string{
		// see: https://github.com/nodejs/node#release-keys
		"nodejs": []string{
			"4ED778F539E3634C779C87C6D7062848A1AB005C", // Beth Griggs <bgriggs@redhat.com>
			"94AE36675C464D64BAFA68DD7434390BDBE9B9C5", // Colin Ihrig <cjihrig@gmail.com>
			"74F12602B6F1C4E913FAA37AD3A89613643B6201", // Danielle Adams <adamzdanielle@gmail.com>
			"71DCFD284A79C3B38668286BC97EC7A07EDE3FC1", // James M Snell <jasnell@keybase.io>
			"8FCCA13FEF1D0C2E91008E09770F7A9A5AE15600", // Michaël Zasso <targos@protonmail.com>
			"C4F0DFFF4E8C1A8236409D08E73BC641CC11F4C8", // Myles Borins <myles.borins@gmail.com>
			"C82FA3AE1CBEDC6BE46B9360C43CEC45C17AB93C", // Richard Lau <rlau@redhat.com>
			"DD8F2338BAE7501E3DD5AC78C273792F7D83545D", // Rod Vagg <rod@vagg.org>
			"A48C2BEE680E841632CD4E44F07496B3EB3C1762", // Ruben Bridgewater <ruben@bridgewater.de>
			"108F52B48DB57BB0CC439B2997B01419BD92F80A", // Ruy Adorno <ruyadorno@hotmail.com>
			"B9E2F5981AA6E0CD28160D9FF13993A75599653C", // Shelley Vohr <shelley.vohr@gmail.com>
		},
		// see: https://classic.yarnpkg.com/en/docs/install/#debian-stable
		"yarn": []string{
			"72ECF46A56B4AD39C907BBB71646B01B86E50310",
		},
	}
	cl := hkp.New(hkp.WithSksKeyserversPool())
	for name, ids := range keys {
		buf, err := cl.GetKeys(ctx, ids...)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(name+".pub", buf, 0644); err != nil {
			return err
		}
	}
	return nil
}
