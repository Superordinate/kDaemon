FROM scratch
EXPOSE 1337 13337
ENTRYPOINT ["env2conf.sh"]
