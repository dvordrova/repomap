# Examples of talking to kvd. `make` only lists them; `make ping` builds one.
CFLAGS = -O2 -Wall -I..

default:
	@echo 'Please choose an example: ping'

ping: ping.c
	$(CC) $(CFLAGS) -o ping ping.c ../net.c
