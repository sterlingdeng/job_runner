#!/bin/sh

for i in {1..50};
do
  echo ${RANDOM} | md5sum
  sleep 0.1
done

