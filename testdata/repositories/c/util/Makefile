# Examples of talking to kvd. `make` only lists them; `make ping` builds one.
# `make watch-replay` links a fixed clock in place of the event loop's.
CFLAGS = -O2 -Wall -I..

default:
	@echo 'Please choose an example: ping watch watch-replay'

ping: ping.c
	$(CC) $(CFLAGS) -o ping ping.c ../net.c

watch: watch.c
	$(CC) $(CFLAGS) -o watch watch.c ../net.c ../loop.c

watch-replay: watch.c fixedclock.c
	$(CC) $(CFLAGS) -o watch-replay watch.c ../net.c fixedclock.c
