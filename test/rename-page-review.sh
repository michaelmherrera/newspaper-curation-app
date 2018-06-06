#!/bin/bash
./makemine.sh
lastdir=""
for file in $(find fakemount/page-review/ -name "*.pdf" | sort); do
  dir=${file%/*}
  if [[ $dir != $lastdir ]]; then
    c=0
    lastdir=$dir
  fi
  let c=c+1
  newfile=$(printf "%04d.pdf" $c)
  mv $file $dir/$newfile
done
