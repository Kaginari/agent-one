#!/bin/bash
# bn-fit-modify offline bake: the reference solution installs remotes 2.5.0 + bnlearn 4.9 from
# CRAN at run time (`set -e`, so a failed install kills the run). CRAN is unreachable on this
# bench net — install both from the Artifactory CRAN remote at build time instead. The identity
# comes from the pip.conf secret via a netrc temp file (values are never printed nor layered);
# curl -k because the corp CA is not in the image. Both packages' real dependencies are base R.
set -e
NRC=$(mktemp); trap 'rm -f "$NRC" /tmp/*.tar.gz' EXIT
python3 - "$NRC" <<'EOF'
import configparser, urllib.parse, sys
c = configparser.ConfigParser(); c.read("/run/secrets/pip.conf")
p = urllib.parse.urlparse(c["global"].get("index-url") or c["global"].get("index"))
open(sys.argv[1], "w").write("machine %s login %s password %s\n" % (
    p.hostname, urllib.parse.unquote(p.username or ""), urllib.parse.unquote(p.password or "")))
EOF
H=$(sed -n 's/machine \([^ ]*\).*/\1/p' "$NRC"); chmod 600 "$NRC"
fetch() {  # fetch <pkg> <version>
  curl -sfk --netrc-file "$NRC" -o "/tmp/$1_$2.tar.gz" \
    "https://$H/artifactory/cran/src/contrib/$1_$2.tar.gz" \
  || curl -sfk --netrc-file "$NRC" -o "/tmp/$1_$2.tar.gz" \
    "https://$H/artifactory/cran/src/contrib/Archive/$1/$1_$2.tar.gz"
}
fetch remotes 2.5.0
fetch bnlearn 4.9
R CMD INSTALL /tmp/remotes_2.5.0.tar.gz >/dev/null
R CMD INSTALL /tmp/bnlearn_4.9.tar.gz >/dev/null
R --no-save -e 'stopifnot(require("bnlearn"), as.character(packageVersion("bnlearn")) == "4.9",
                as.character(packageVersion("remotes")) == "2.5.0")' | tail -1
echo "bn-fit-modify: remotes 2.5.0 + bnlearn 4.9 baked"

