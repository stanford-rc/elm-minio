#!/bin/bash

if [ -z "${SSH_AUTH_SOCK}" ]; then
	echo "No SSH_AUTH_SOCK environment variable has been set (see ssh-agent(1) manual page)";
	exit 1;
elif [ ! -e "${SSH_AUTH_SOCK}" ]; then
	echo "The specified SSH_AUTH_SOCK does not exist: ${SSH_AUTH_SOCK}";
	exit 1;
elif [ ! -O "${SSH_AUTH_SOCK}" ]; then
	echo "You do not appear to own the specified SSH_AUTH_SOCK: ${SSH_AUTH_SOCK}";
	exit 1;
elif ssh-add -l 2>&1 | grep -q "Could not open a connection to your authentication agent."; then
	echo "Could not open a connection to your authentication agent.";
	echo "Ensure your ssh-agent is running and reset SSH_AUTH_SOCK to connect to it.";
	exit 1;
elif ssh-add -l 2>&1 | grep -q "The agent has no identities."; then
	echo "The agent has no identities.";
	echo "Use ssh-add to add your SSH identity the ssh-agent for $SSH_AUTH_SOCK";
	exit 1;
fi

exit 0;
