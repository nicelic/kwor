# kwor User Manual

> This manual is written for first-time users, beginners, and non-technical operators.
>
> It focuses on how to use the project, what to configure first, and what each change affects.
>
> 中文版: [使用手册.md](./%E4%BD%BF%E7%94%A8%E6%89%8B%E5%86%8C.md)

---

## 1. What this project is, in plain language

`kwor` is a web-based control panel for managing proxy services.

The simplest way to understand it is:

> **A web console for managing inbound entries, user accounts, subscriptions, TLS certificates, and some system-level features.**

It is not just a single page. It combines:

- The web admin panel
- A sing-box configuration chain
- A mihomo configuration chain
- Subscription distribution
- TLS / certificate management
- Traffic statistics
- Firewall management
- Port forwarding
- Reverse proxy
- System optimization
- Local machine monitoring

Because of that, new users should not try to configure everything at once.

---

## 2. The 8 most important ideas to remember first

### 2.1 The panel address is not the subscription address

- **Panel address**: where you log in to manage the system
- **Subscription address**: what clients use to import or update configurations

They are separate settings:

- The panel has its own port, path, and domain
- The subscription service has its own port, path, and domain

So:

- Changing the panel address does not automatically change the subscription address
- Changing the subscription address does not automatically change the panel login address

---

### 2.2 Inbounds are your entry points

An inbound means:

> What protocol, what port, and what TLS mode users connect to when they enter the server.

Examples:

- VLESS on port 443
- Hysteria2 on port 8443
- Socks / HTTP / Mixed on another port

---

### 2.3 Users are accounts

The Users page manages:

- Username
- UUID / password / identity fields
- Total traffic quota
- Expiration time
- User groups
- Which inbounds each user can use

Simple version:

- Inbounds define how the door is opened
- Users define who can enter, for how long, and with how much traffic

---

### 2.4 Outbounds are your exits

Outbounds define:

> Where the traffic goes after it leaves the inbound side.

Routing and DNS behavior both depend on outbounds.

---

### 2.5 sing-box and mihomo are two parallel chains

In the left menu you will see two similar groups of pages:

- sing-box: Inbounds, Users, Outbounds, TLS, Routing, DNS
- mihomo: Inbounds, Users, Outbounds, TLS, Routing, DNS

These are not duplicate decorations. They are two separate configuration chains.

Best advice for beginners:

- Start with only one chain
- Either use sing-box first
- Or use mihomo first
- Do not configure both at the same time on day one

---

### 2.6 TLS / HTTPS means encrypted access

TLS affects two kinds of addresses:

- Whether the panel uses `https://`
- Whether the subscription service uses `https://`

If you do not have a domain or a public certificate yet, you can start with HTTP first and add HTTPS later.

---

### 2.7 The Settings page is not a single simple form

This is one of the most important things to understand.

The Settings page is not a place where everything is saved the same way.

There are two groups:

#### Group 1: uses the top save button

These tabs use the shared top action bar:

- Interface
- Subscription
- JSON Subscription
- Clash Subscription
- Language

#### Group 2: each page saves by itself

These tabs have their own buttons and their own save logic:

- Traffic Management
- Firewall
- Port Forwarding
- Optimization
- Certificate Center
- Reverse Proxy
- Kernel Management
- Monitor

So:

- On the first 5 tabs, watch the top save area
- On the last 8 tabs, watch the controls inside the current page

---

### 2.8 Some settings affect the whole machine, not just the panel

Some actions are much stronger than a normal save:

- Changing the SSH port restarts SSH
- Enabling the firewall rebuilds managed rules
- Changing Linux DNS rewrites `resolv.conf`
- sysctl optimization applies kernel parameters immediately
- MTU optimization creates scripts and tries to keep them persistent
- Installing or removing ACME, vnstat, or nftables affects the system environment

Best habit:

> **Change one small thing at a time, then verify it.**

---

## 3. The best order for your first use

### 3.1 Step 1: make sure the panel opens normally

First solve this:

- Can you open the panel?
- Can you log in?
- Does the address work after login?

Typical defaults are usually:

- Panel port: `8888`
- Panel path: `/app/`
- Subscription port: `22780`

The subscription path is usually generated randomly on first initialization.

---

### 3.2 Step 2: after login, look at the Home page first

The Home page is your dashboard.

Use it to confirm:

1. The panel itself is running
2. sing-box is running or not
3. mihomo is running or not
4. The system info looks normal
5. Logs and backup entry points are available

You can also do these actions there:

- Start sing-box
- Stop sing-box
- Restart sing-box
- Start mihomo
- Stop mihomo
- Restart mihomo
- Open logs
- Open backup / restore

Beginner advice:

- If something stops working after configuration, go back to Home first
- Check whether the actual core is running before anything else

---

### 3.3 Step 3: only change the most basic settings first

After the first login, start with:

1. `Settings -> Interface`
2. `Settings -> Subscription`
3. `Settings -> Language`

Fix addresses, ports, and language first.

---

### 3.4 Step 4: only then start business configuration

If you are using sing-box, the best order is:

1. TLS
2. Inbounds
3. Users
4. Outbounds
5. Routing
6. DNS
7. Subscription Manager

If you are using mihomo, the best order is:

1. mihomo TLS
2. mihomo Inbounds
3. mihomo Users
4. mihomo Outbounds
5. mihomo Routing
6. mihomo DNS
7. Subscription Manager

---

### 3.5 Step 5: leave advanced system features for later

Do not rush into these pages:

- Traffic Management
- Firewall
- Port Forwarding
- Optimization
- Advanced certificate settings
- Reverse Proxy
- Kernel Management
- Monitor retention tuning

They are powerful, but they can affect the whole system if configured incorrectly.

---

## 4. How to understand the left-side menu

### 4.1 Home

What it is for:

- Overall status
- System charts
- Start / stop / restart cores
- Logs
- Backup

Who uses it:

- Everyone

---

### 4.2 Subscription Manager

What it is for:

- Organizing subscription output
- Generating subscription links
- Generating QR codes
- Building groups

Who uses it:

- Anyone who wants to distribute one clean import link to clients

Important:

- It is not where you create inbounds
- It is not where you create user accounts

---

### 4.3 Inbounds

What it is for:

- Opening protocol entry points
- Setting ports
- Setting TLS
- Checking port status
- Viewing port logs

Plain version:

> This is where you open a door on the server.

---

### 4.4 Users

What it is for:

- Creating accounts
- Bulk account creation
- Traffic quota
- Expiration time
- Binding users to inbounds
- QR codes and charts

Plain version:

> This is where you issue access passes.

---

### 4.5 Outbounds

What it is for:

- Defining egress targets
- Grouping exits
- Watching outbound status

Plain version:

> This is where traffic goes after it leaves.

---

### 4.6 Routing

What it is for:

- Deciding which traffic goes where
- Choosing direct, proxy, block, or split routing

Plain version:

> These are your traffic rules.

---

### 4.7 DNS

What it is for:

- Domain resolution behavior
- DNS server selection
- DNS rule logic

Plain version:

> This decides how domain names are resolved and which DNS is used in different situations.

---

### 4.8 Basics

What it is for:

- Logging settings
- NTP
- Experimental features
- Some Clash API / V2Ray API options

For beginners:

- This is not a must-change page
- It is more of a low-level runtime page

---

### 4.9 Admins

What it is for:

- Admin accounts
- Password changes
- Login history
- Change history
- API tokens

Best beginner advice:

- Change the default password as soon as possible

---

### 4.10 Settings

What it is for:

- Global settings
- Advanced system features

Important:

- This page is the easiest place to improve the system
- It is also the easiest place to break access if used carelessly

---

## 5. The most common beginner workflow

## 5.1 If you only want one working sing-box account first

Use this order:

1. Confirm normal status on Home
2. Confirm panel and subscription addresses in Settings
3. Prepare TLS
4. Create one inbound
5. Create one user
6. Bind that user to that inbound
7. View the user QR code or link
8. Use Subscription Manager only if you want a packaged subscription output

---

## 5.2 If you only want one working mihomo account first

Use this order:

1. Confirm normal status on Home
2. Confirm panel and subscription settings
3. Configure mihomo TLS
4. Create one mihomo inbound
5. Create one mihomo user
6. Configure mihomo outbounds
7. Configure mihomo routing
8. Configure mihomo DNS
9. Use Subscription Manager

Special reminder:

The `mihomo DNS` page mainly configures how mihomo resolves names. It is not simply an extra system-wide DNS service page.

---

## 6. Detailed guide to the Settings page

This is the most important part of the manual.

---

## 6.1 Settings -> Interface

This tab controls how the panel itself is accessed.

Changes here usually use the top shared save button.

### Address `webListen`

This is the **panel listening address**, not the public domain you share.

Common meanings:

- Empty: default listening behavior
- `127.0.0.1`: local-only
- `0.0.0.0`: external access allowed

Beginner advice:

- If you do not understand it, leave it alone

---

### Port `webPort`

This is the panel login port.

Examples:

- `8888`
- `443`

What happens after changing it:

- The old port stops working
- You must reopen the panel using the new port

Beginner advice:

- The default port is usually safer for first-time use
- Before switching to `80` or `443`, make sure nothing else is using that port

---

### Panel Path `webPath`

This is the URL path of the panel.

Examples:

- `/app/`
- `/panel/`

What happens after changing it:

- The old path no longer works
- You must open the new path

Beginner advice:

- Keeping `/app/` is the safest choice
- If you change it, keep the leading and trailing slash style

---

### Domain `webDomain`

This is the public-facing domain used when the panel builds display links.

When to fill it:

- You have a real domain
- You plan to use HTTPS
- You want domain-based access instead of raw IP

Do not confuse:

- `webListen` = what the process binds to
- `webDomain` = what users should see

---

### Panel URI `webURI`

This is a helper field for the full panel address.

Beginner advice:

- Leave it empty if you are not sure
- Let the system build the address from domain, port, and path first

---

### Session timeout `sessionAge`

This controls how long login sessions stay valid.

Simple version:

- Lower value: logs out sooner
- Higher value: stays logged in longer

---

### Traffic retention `trafficAge`

This influences some traffic history retention behavior.

Simple version:

- Longer value: more history
- Also more stored data

---

### Time zone `timeLocation`

This affects:

- Expiration time display
- Chart timestamps
- Log timestamps
- Some scheduled behavior

Example commonly used in mainland China:

- `Asia/Shanghai`

---

### Version check / install area

This area can:

- Show the local version
- Show remote versions
- Check updates
- Select a version
- Install a version

Beginner advice:

- Fine for manual binary-style deployments if you understand what you are doing
- Do not assume it is the right upgrade path for Docker or special deployment methods
- If the install button is unavailable, that usually means the current environment should not be updated that way

---

## 6.2 Settings -> Subscription

This tab controls how clients receive subscriptions.

Changes here also use the top shared save button.

### Base64 encoding `subEncode`

Many clients expect subscription content in Base64 form.

Beginner advice:

- Keep it enabled if you are unsure

---

### Show user info `subShowInfo`

This usually controls whether traffic / expiration hints are included in subscription output.

Good when:

- You want client-side subscription information to be richer

---

### Address `subListen`

This is the **subscription service listening address**.

It is similar in concept to the panel listen address, but for subscription traffic.

---

### Port `subPort`

This is the subscription service port.

The common default is usually:

- `22780`

What happens after changing it:

- Old subscription links may stop working
- Old QR codes may stop working
- Clients may need new import addresses

---

### Domain `subDomain`

This is the public-facing domain for subscription links.

Useful when:

- You want to share a domain instead of an IP
- You enabled HTTPS for subscriptions

---

### Path `subPath`

This is the subscription path.

On first initialization it is usually generated randomly.

Strong advice:

- Do not replace it with something too simple
- Avoid very guessable values like `/sub/`

What happens after changing it:

- Old subscription URLs and QR codes may become invalid

---

### Update interval `subUpdates`

This is more like a suggested client refresh interval.

Beginner advice:

- Keep the default unless you have a clear reason

---

### Subscription URI `subURI`

Like the panel URI, this is a helper for the full subscription address.

Beginner advice:

- Leave it empty if you are not sure

---

## 6.3 Settings -> JSON Subscription

This tab is not the first layer of “can the service work.”

It is more like:

> The advanced template used when generating sing-box style JSON subscriptions.

Changes here use the top shared save button.

There is also a dedicated reset button:

- `Reset JSON Subscription`

It resets this template page only.

### Who should use this page

- Users whose base chain is already working
- Users who want to customize generated JSON subscription output

### Beginner advice

- If your base chain is not working yet, do not start here

### Typical things you will see here

- TLS store choices
- Log template settings
- DNS template logic
- fakeip
- bootstrap DNS
- inbound / TUN templates
- route set sources
- latency test behavior
- sniff / hijack-dns / external controller options

### Common risk here

- The node may still “work,” but client behavior becomes strange
- DNS looks partly fine, but some domains break
- Generated JSON does not match your expectation

---

## 6.4 Settings -> Clash Subscription

This page is similar to JSON Subscription, but targets:

- Clash-style output
- Mihomo-style output

It also uses the top shared save button.

There is also a dedicated reset button:

- `Reset CLASH Subscription`

That reset only applies to this template page.

### Typical things you will see here

- mixed-port
- allow-lan
- external-controller
- unified-delay
- tcp-concurrent
- TUN options
- fake-ip / fallback / nameserver
- rule set templates
- sniffer
- hosts
- UDP blocking and advanced behavior

### Beginner advice

- If you have not already understood outbounds, routing, and DNS, do not heavily customize this page yet

### Common risk here

- Not total failure
- Instead, “strange behavior,” such as LAN exposure, odd Fake-IP behavior, or incorrect DNS logic

---

## 6.5 Settings -> Language

This page is simple:

- Choose the language you understand best

Beginner advice:

- Do that early, before going deeper into the panel

---

## 6.6 Settings -> Traffic Management

This page does not use the top shared save button.

It manages:

- vnstat installation and removal
- Traffic statistics enable / disable
- Monthly traffic limit
- Monthly reset day
- Period reset
- Total reset

### What you will see here

- Current traffic status
- Whether vnstat is installed
- Version, path, and data directory
- Current upload, download, and total traffic
- Monthly usage progress

### Important actions

- Download / remove vnstat
- Toggle traffic statistics
- Save traffic settings
- Reset traffic
- Reset total traffic

### Important effects

- If traffic statistics are disabled, new traffic is usually not counted during that time
- Removing vnstat is not just clearing one UI section; it may remove system-level traffic data as well

### When to use it

- When you want host-level traffic tracking
- When you want monthly reset behavior

---

## 6.7 Settings -> Firewall

This is a high-risk page.

It does not use one shared save button. Most actions apply immediately.

### What this page can do

- Install nftables
- Toggle the managed firewall
- Change the SSH port
- Toggle SSH proxy capability
- Manage reserved system ports
- Create normal rules
- Create GeoIP rules
- Set GeoIP refresh intervals
- Trigger GeoIP hot refresh

### The 4 most important questions here

1. Will the panel port stay open?
2. Will the SSH port stay open?
3. Will the subscription port stay open?
4. Will GeoIP rules accidentally block you?

### What normal rules usually define

- Protocol
- Port or port range
- IPv4 / IPv6 / dual stack
- Source IP / CIDR

### What GeoIP rules usually define

- Which countries are allowed
- Which countries are blocked
- Which ports use GeoIP matching
- Rule source and refresh behavior

### Very important warning

- Before enabling the firewall, make sure SSH and panel access will remain available
- Changing the SSH port restarts SSH
- Keep an existing login session open while you test changes

---

## 6.8 Settings -> Port Forwarding

This page also saves inside its own interface.

It is used for port forwarding.

### Good use cases

- Forwarding a local port to another IP / port
- Simple traffic relay
- Range or multi-port mapping

### What you will configure

- Protocol
- IPv4 / IPv6
- Local port
- Target IP
- Target port
- Rate limit
- Single / range / multi-port mode

### Beginner advice

- Start with one simple rule
- First verify that the target service is actually reachable
- If forwarding fails, the target service itself may be the problem

---

## 6.9 Settings -> Optimization

This is a system-level page, not a cosmetic optimization page.

It also works with page-specific actions, not the shared top save button.

### It has 4 main sections

#### 1. Disable system logs

This affects:

- systemd journal persistence
- configuration file rebuilding
- journald restart

Use it only if:

- You clearly understand why you want to reduce system logging

#### 2. sysctl optimization

This affects:

- `/etc/sysctl.conf`
- `/etc/sysctl.d/...`
- immediate kernel parameter application

Use it only if:

- You understand the network/kernel parameters you want

#### 3. Linux DNS

This affects:

- `resolv.conf`
- system nameserver settings

Use it only if:

- You know how you want the host DNS configured

#### 4. MTU optimization

This affects:

- MTU on the default interface
- generated scripts
- systemd startup behavior

Use it only if:

- You know MTU tuning is helpful for your line or environment

### The overall advice for this page

- If you are not sure, do not use it yet
- Wrong changes here affect the whole machine network

---

## 6.10 Settings -> Certificate Center

This is one of the strongest pages in the project.

It uses its own actions instead of the top shared save button.

### What this page can do

- Install / remove acme.sh
- Issue public certificates
- Create self-signed certificates
- Manage ACME accounts
- Manage DNS accounts
- View the certificate list
- Renew certificates
- Force renew certificates
- Toggle auto renewal
- Push certificates to local directories
- Apply certificates to the panel
- Apply certificates to subscription service
- View issue / renew logs

### The 3 most common beginner scenarios

#### Scenario 1: I have a domain and want a real certificate

Recommended method:

- DNS validation

Requirements:

- You control the domain DNS
- You have the proper DNS provider credentials or API details

#### Scenario 2: I do not have DNS API, but port 80 is available

Possible methods:

- HTTP standalone
- HTTP webroot

#### Scenario 3: I only want to test HTTPS first

Use:

- Self-signed certificates

### The 3 things you must separate clearly

1. A certificate existing is not the same as it being applied
2. Panel HTTPS and subscription HTTPS can be applied separately
3. A self-signed certificate can work even if browsers do not trust it

### Practical beginner advice

- For real public usage: prefer real certificates
- For temporary testing: self-signed is fine
- After applying certificates, go back and re-check panel and subscription addresses

---

## 6.11 Settings -> Reverse Proxy

This page also saves through its own actions.

Its purpose is:

> Receive requests on one side and forward them to a target service based on protocol, port, host, and path.

### What you usually configure here

- Listen protocol
- Listen port
- Hosts
- Path Prefix
- Target protocol
- Target address
- Target port
- Target path
- IP strategy
- HTTP version strategy
- Upstream TLS verification
- Bound certificate

### The easiest mistake for beginners

You must separate:

#### The listen side

- How outside traffic reaches you

#### The target side

- Where you forward that traffic

### Good use cases

- Forwarding a domain entry to an internal web service
- Path-based service splitting
- Building one shared entry point for multiple services

### Important warning

- Do not forward traffic back into yourself and create a loop
- If the listen side uses HTTPS, bind the correct certificate
- Keep path prefix and target path logic clearly separated

---

## 6.12 Settings -> Kernel Management

This is a Linux kernel management page, not a proxy configuration page.

### What it can do

- Select a kernel provider
- Select a version
- Select an architecture
- Download kernel packages
- Install kernel packages
- Reboot
- Scan old kernel packages
- Clean up old kernel packages

### Who should use it

- Only users who intentionally want to manage Linux kernels

### Beginner advice

- Most beginners can ignore this page completely
- This is not a “small panel feature”; it changes the operating system kernel

---

## 6.13 Settings -> Monitor

This is the local system monitoring page.

It also saves through page-specific controls.

### What it shows

- CPU
- Memory
- Disk read / write
- Network up / down
- History charts
- Current sample status
- Monitoring database size

### What it lets you change

- Sampling interval
- Short-term retention
- Archive retention
- Clearing monitoring history

### How to understand those settings

#### Sampling interval

- How often data is collected

#### Short-term retention

- How long fine-grained recent history is kept

#### Archive retention

- How long older coarse-grained history is kept

### What happens if you clear monitor history

- Historical charts are removed
- Monitoring history inside the monitor database is cleaned
- The main panel database is not deleted
- Your core panel settings stay intact

### Beginner advice

- If you do not need ultra-detailed history, do not set overly aggressive sampling

---

## 7. How to use a few other common pages

## 7.1 TLS

This page mainly manages:

- TLS configuration pools
- Which TLS setup an inbound should use

Beginner advice:

- First make sure the basic certificate, domain, and handshake are correct
- Leave advanced TLS behavior for later

---

## 7.2 Routing

The two key questions are:

1. What is the default outbound?
2. Which traffic should use exceptions?

Beginner advice:

- Start with a small rule set
- Make sure the default outbound is correct
- If “everything breaks,” check the default outbound first

---

## 7.3 DNS

This page often decides whether the service feels “mostly fine” or actually works properly.

Beginner advice:

- Start with a small, clear DNS setup
- Change one part at a time
- Test a few common domains after each change

---

## 7.4 Basics

This page mostly contains:

- Logging settings
- NTP
- Experimental features

Beginner advice:

- If you are just trying to use the project normally, defaults are often enough

---

## 7.5 Admins

This page is simple but important:

- Change passwords
- Check records
- Manage tokens

Practical advice:

- Change default credentials early
- Store tokens safely once generated

---

## 8. The most common mistakes

## 8.1 I changed the panel port or path and now the old address is dead

That is expected.

You changed the actual entrance itself.

For example:

- `8888` to `9999`
- `/app/` to `/panel/`

The old address will stop working.

---

## 8.2 The panel still opens, but subscriptions stopped working

That happens because panel access and subscription access are separate.

Check:

- `subPort`
- `subPath`
- `subDomain`
- `subURI`

---

## 8.3 I enabled the firewall and locked myself out

Typical reasons:

- The panel port was not allowed
- The SSH port was not allowed
- The subscription port was not allowed
- A GeoIP rule blocked your own access

---

## 8.4 I enabled HTTPS, but the browser still says unsafe

If you are using a self-signed certificate, that is normal.

It does not mean the panel is broken.

It means the browser does not trust that certificate.

---

## 8.5 Advanced settings made everything more confusing

That usually happens because these pages are not beginner-first pages:

- JSON Subscription
- Clash Subscription
- Firewall
- Optimization
- Kernel Management

The safest order is always:

1. Make the base chain work
2. Customize advanced templates later
3. Touch system-level features last

---

## 8.6 Do not casually use panel-side install buttons in Docker environments

Many Docker deployments should be upgraded by:

- Pulling a newer image
- Recreating the container

Not by replacing program files from inside the panel.

---

## 9. The shortest memory method for complete beginners

If you only want to remember the essentials, remember these 8 lines:

1. Home shows status
2. Settings changes addresses
3. TLS handles certificates
4. Inbounds open entry points
5. Users create accounts
6. Outbounds define exits
7. Routing and DNS shape traffic behavior
8. Subscription Manager handles distribution

---

## 10. Final practical advice for beginners

### Advice 1: start with only one chain

Use sing-box first, or use mihomo first.

---

### Advice 2: change one small thing at a time

The best rhythm is:

1. Change one page
2. Save
3. Verify
4. Move to the next page

---

### Advice 3: write down old address values before changing them

Especially:

- Panel port
- Panel path
- Subscription port
- Subscription path
- Domain values

---

### Advice 4: treat high-risk pages like system tools

These pages can affect the whole machine:

- Firewall
- Optimization
- Certificate Center
- Reverse Proxy
- Kernel Management

---

### Advice 5: if you are unsure, stay conservative

This project has many features, but you do not need all of them on day one.

Very often:

> **A stable working setup is better than turning on every option.**

---

## 11. What to add next if you want an even better onboarding set

The next three documents would help new users a lot:

1. `5-Minute Quick Deployment Guide`
2. `Minimal sing-box Setup Guide`
3. `Minimal mihomo Setup Guide`

That would make first-time onboarding much easier.
