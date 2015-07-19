FROM scratch
MAINTAINER Nick Owens <mischief@offblast.org>

ADD bin/anchor /anchor

ENTRYPOINT ["/anchor"]

