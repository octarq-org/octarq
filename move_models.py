import os

with open("internal/models/models.go", "r") as f:
    lines = f.readlines()

moves = {
    "dns": ["ProviderAccount", "Domain"],
    "links": ["Link", "LinkEvent", "RoutingRules", "RoutingRule"],
    "mail": ["Mailbox", "Email", "Attachment", "SMTPSender", "MessageIDHeader"],
}

out_files = {
    "dns": ["package dns\n\nimport (\n\t\"encoding/json\"\n\t\"time\"\n)\n\n"],
    "links": ["package links\n\nimport (\n\t\"database/sql/driver\"\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"time\"\n)\n\n"],
    "mail": ["package mail\n\nimport (\n\t\"database/sql/driver\"\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"time\"\n)\n\n"]
}

struct_lines = []
skip_indices = set()

# Helper to extract a type or func
def extract_block(start_idx):
    block = []
    braces = 0
    in_block = False
    idx = start_idx
    
    # backtrack for comments
    while idx > 0 and lines[idx-1].startswith("//"):
        idx -= 1
        
    start_capture = idx
    idx = start_idx
    
    while idx < len(lines):
        line = lines[idx]
        block.append(line)
        braces += line.count('{') - line.count('}')
        if '{' in line:
            in_block = True
        if in_block and braces == 0:
            break
        # Some types don't have braces (e.g. type RoutingRules []RoutingRule)
        if not in_block and not '{' in line and not line.strip().endswith('struct') and line.strip().startswith('type'):
            if '(' not in line: # not a var block
                break
        idx += 1
        
    return start_capture, idx, block

to_move_all = []
for p, structs in moves.items():
    to_move_all.extend(structs)

i = 0
new_lines = []
while i < len(lines):
    line = lines[i]
    moved = False
    
    # Check for type definition
    if line.startswith("type "):
        parts = line.split()
        if len(parts) >= 2:
            name = parts[1]
            if name in to_move_all:
                start, end, block = extract_block(i)
                # Find which plugin it belongs to
                plugin = next(p for p, s in moves.items() if name in s)
                out_files[plugin].extend(lines[start:end+1])
                # mark as skipped for new_lines
                for j in range(start, end+1):
                    skip_indices.add(j)
                i = end
                moved = True
                
    # Check for TableName / methods of moved structs
    elif line.startswith("func ("):
        # e.g. func (d *Domain) TableName()
        parts = line.split(")")
        if len(parts) > 0:
            receiver = parts[0] # "func (d *Domain"
            for struct in to_move_all:
                if struct in receiver:
                    start, end, block = extract_block(i)
                    plugin = next(p for p, s in moves.items() if struct in s)
                    out_files[plugin].extend(lines[start:end+1])
                    for j in range(start, end+1):
                        skip_indices.add(j)
                    i = end
                    moved = True
                    break
    
    if not moved and i not in skip_indices:
        pass
    i += 1

for j, line in enumerate(lines):
    if j not in skip_indices:
        # Also remove them from AllModels
        skip_allmodels = False
        for struct in to_move_all:
            if f"&{struct}{{}}" in line:
                skip_allmodels = True
        if not skip_allmodels:
            new_lines.append(line)

with open("internal/models/models.go", "w") as f:
    f.writelines(new_lines)

for p, content in out_files.items():
    with open(f"plugins/{p}/models.go", "w") as f:
        f.writelines(content)
