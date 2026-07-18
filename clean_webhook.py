with open("plugins/mail/webhook.go", "r") as f:
    lines = f.readlines()

new_lines = []
skip = False
for line in lines:
    if line.startswith("type mailboxDTO struct {"):
        skip = True
    elif line.startswith("type bounceEvent struct {"):
        skip = True
    elif line.startswith("func isAWSSNSURL"):
        skip = True
    elif line.startswith("func extractBounceEvents"):
        skip = True

    if skip and line.strip() == "}":
        skip = False
        continue
    
    if not skip:
        # replace remaining h. with p.
        line = line.replace("h.emitEmail", "p.emitEmail")
        line = line.replace("h.getSetting", "p.ctx.GetGlobalSetting")
        line = line.replace("h.url(r, ", "p.url(r, ") 
        line = line.replace("secureEqual", "subtle.ConstantTimeCompare")
        line = line.replace("keyCatchAll", '"catch_all_mailbox"')
        new_lines.append(line)

with open("plugins/mail/webhook.go", "w") as f:
    f.writelines(new_lines)
