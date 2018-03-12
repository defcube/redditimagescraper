RedditImageScraper fetches high resolution images from reddit and places them into a directory.  I use it to so that I always have high res images for my desktop.

Example Usage (in the form of a bash script that I use for myself)

```
#!/usr/bin/env bash -e
tmp=`mktemp -d`
dst=~/Dropbox/RedditSync
mkdir -p $dst
redditimagescraper --minwidth=5120 --minheight=2880 $tmp EarthPorn AnimalPorn InfrastructurePorn CityPorn skylineporn Cyberpunk MilitaryPorn RoadPorn TechnologyPorn StarshipPorn DestructionPorn waterporn spaceporn geologyporn WeatherPorn winterporn RoomPorn FractalPorn ImaginaryBestOf ImaginaryWastelands ViewPorn futureporn ImaginaryMindscapes WarshipPorn HeavySeas sfwporn MachinePorn MacroPorn MicroPorn SkyPorn itookapicture ImaginaryCyberpunk WarplanePorn chinafuturism
rm -f $dst/*.jpg
mv $tmp/* $dst
rmdir $tmp
```
