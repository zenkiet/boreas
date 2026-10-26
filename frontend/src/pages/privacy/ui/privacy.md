# Privacy Policy

**Last updated:** 26 October 2026

This policy explains what the **Boreas** app does with data. It is published by
**Kiet Le**, an individual developer, who is the data controller for
the limited processing described here.

## The short version

We run no servers. Boreas talks only to the server address you type in, and
everything it stores stays on your device. Three exceptions are described
below: push notifications go through Google, crash reports reach us through
Google, and Bora's voice downloads its files from Hugging Face.

## 1. What stays on your device

The App stores the following on the device, and nowhere else:

| Stored value             | What it is                                 | Why                                        |
| ------------------------ | ------------------------------------------ | ------------------------------------------ |
| `boreas-server`          | The server address you entered             | so the App reconnects without asking again |
| `boreas-token`           | Your session token for that server         | to stay signed in                          |
| `boreas-theme`           | Light, dark or automatic                   | for appearance                             |
| `boreas-log-level`       | How much the App logs to the console       | only if set by hand, for troubleshooting   |
| `boreas-welcomed`        | Whether you finished the intro             | so it is not shown twice                   |
| `boreas-pins`            | Projects you pinned to the sidebar         | to keep them there                         |
| `boreas-fleet`           | Your projects and tasks as last seen       | to show Home at once when the App opens    |
| `boreas-recent-tasks`    | The last five tasks you opened from Search | to offer them again                        |
| `boreas-monitor`         | Last CPU, memory and network samples       | to draw the chart before new data arrives  |
| `boreas-live-folded`     | Whether you folded or opened Home's chart  | to keep it that way next time              |
| `boreas-push`            | Whether you enabled notifications          | to remember your choice                    |
| `boreas-push-registered` | Whether the device registered for push     | to avoid registering twice                 |
| `boreas-bora`            | Bora's settings: language, voice, speed    | to keep your choices                       |
| `boreas-lock`            | Whether the App lock is on                 | to ask for biometrics or your passcode     |
| `boreas-env-offers`      | Variables you chose not to route           | so the offer is not shown again            |

If you download Bora's voice, its files are kept in the App's private storage
too. When the App lock is on, your device checks your face, fingerprint or
passcode; the App only learns whether the check passed.

Uninstalling the App removes all of it.

## 2. What goes to your own server

When you sign in and use the App, it sends requests to the server address you
chose. That traffic contains your username and password at sign-in, your
session token afterwards, and whatever you do in the App — creating tasks,
editing environment variables, reading logs.

A photo or screenshot you attach to a question for Bora goes the same way, and
only when you attach one. The App first shrinks it on the device, which drops
its location and other metadata. Your server keeps it in the storage its
operator chose and passes it to the AI model the operator set up for Bora.

That server is yours. We have no access to it, and this policy does not cover
what it logs or stores. If you did not set it up yourself, ask whoever did.

## 3. Push notifications (Google)

If you turn on notifications, the App registers with **Firebase Cloud
Messaging**, a Google service, and receives a device push token. That token is
sent to your Boreas server so it can deliver alerts to your device.

- Google receives the push token and technical information about the device as
  part of operating the service. See Google's privacy policy at
  <https://policies.google.com/privacy>.
- Notification content is composed by your server, not by us.
- You can turn notifications off at any time in **Settings**, or in your
  device's system settings. Turning them off stops the registration.

## 4. Crash reports (Google)

When the App crashes or runs into an error, it sends a report to **Firebase
Crashlytics**, a Google service, so we can fix the problem:

- A report holds the stack trace, the device model, the system and App
  versions, and a random ID for this installation. It carries no account,
  username or session token.
- An error report also holds a short message, such as "Log download failed".
  The App removes your server's address from it first; it can still name the
  project or task involved.
- Google keeps crash reports for 90 days and processes them for us. See
  Google's privacy policy at <https://policies.google.com/privacy>.
- Reports are never used for advertising or tracking.

## 5. Bora's voice (Hugging Face)

On a phone or a tablet, Bora can listen and answer out loud. The voice runs on
your device:

- What you say is turned into text on the device. Only that text is sent, to
  your own server, as your question; audio never leaves the device.
- On iOS and iPadOS 26 or later, Bora listens to English with Apple's on-device
  dictation. Its model is downloaded and kept up to date by the system, from
  Apple, as for the keyboard's dictation; the audio stays on the device.
- Bora reads its answers aloud on the device as well.
- The voice's files are downloaded once, when you choose to, from **Hugging
  Face** (<https://huggingface.co/zenkiet/boreas-voice>). The download carries
  no account or token, but Hugging Face sees your device's IP address, as any
  website would. See its privacy policy at <https://huggingface.co/privacy>.
- You can delete the files at any time in **Settings › Bora**.

## 6. What we do not do

- We do not collect analytics, telemetry, or usage statistics. Crash reports
  (section 4) are the one exception.
- We do not use advertising or tracking of any kind.
- We do not sell or share personal data.
- We do not have accounts. Accounts exist only on your server.

## 7. Children

Boreas is a tool for operating server infrastructure. It is not directed at
children and we do not knowingly collect data from children.

## 8. Your rights

Because the data described in section 1 never leaves your device, you control
it directly: signing out removes your session token, and uninstalling the App
removes all of it. Crash reports carry no account or name, so we cannot tell
which are yours; Google deletes them after 90 days.

## 9. Data on your own server

If you operate a Boreas server for other people, **you** are the controller of
the data on it, including the push tokens their devices send you. Your own
privacy obligations apply to that data.

## 10. Changes

We may update this policy. The current version always ships inside the App and
is published at <https://boreas.zenkiet.dev/legal/privacy>. The date at the top
tells you when it last changed.

## 11. Contact

**Zen Le** — **zenkiet0906@gmail.com**
