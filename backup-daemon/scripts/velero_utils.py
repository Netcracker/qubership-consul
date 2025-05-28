# Copyright 2024-2025 NetCracker Technology Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import sys
import time

import restore


def post_restore():
    attempts = 9
    timeout = 10
    
    while attempts > 0:
        try:
            print("Try to update auth methods")
            restore.Restore("").update_auth_methods()
            return
        except Exception as e:
            attempts -= 1
            print(f"Restore failed due to: {str(e)}, {attempts} attempts left")
            if attempts == 0:
                raise e
            print(f"Wait for {timeout} seconds for next try")
            time.sleep(timeout)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit("Please provide a command name to run")
    command = sys.argv[1]
    if command == "post_restore":
        restore.Restore("").check_consul_leader()
        post_restore()
    else:
        sys.exit(f"Invalid command name: {command}")
