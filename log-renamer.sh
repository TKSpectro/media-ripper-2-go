#!/bin/bash

# This script should be run once per day to move the log file of the previous month to old-logs directory
# The current log file will be name cronlog.txt and the previous month's log file will be renamed to the last month's name

LOG_FILE="$HOME/Applications/media-ripper-2-go/cronlog.log"
LOG_DIR="$HOME/Applications/media-ripper-2-go/logs"

# exit if the log file does not exist without error
if [ ! -f "$LOG_FILE" ]; then
    echo "Log file does not exist: $LOG_FILE"
    exit 0
fi

# If the log directory does not exist, create it
mkdir -p "$LOG_DIR"

# Get the previous month and year
PREVIOUS_MONTH=$(date -d "last month" +"%m")
YEAR_OF_LAST_MONTH=$(date -d "last month" +"%Y")

# Check if the month/year of today is different from yesterday
# If it is, rename the log file
if [ "$(date -d "last day" +"%m")" -ne "$(date +"%m")" ]; then
    mv "$LOG_FILE" "$LOG_DIR/$YEAR_OF_LAST_MONTH-$PREVIOUS_MONTH.log"
fi
