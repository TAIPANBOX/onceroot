.PHONY: test fuzz linux-test mutate teeth

test:
	go test ./...

fuzz:
	@for t in $$(grep -rhoE '^func Fuzz[A-Za-z0-9_]+' --include='*_test.go' . | sed 's/func //'); do \
		pkg=$$(grep -rlE "^func $$t\(" --include='*_test.go' . | xargs dirname); \
		echo "== $$t in $$pkg"; go test -run '^$$' -fuzz "^$$t$$" -fuzztime 60s $$pkg || exit 1; \
	done

linux-test:
	scripts/linux-test.sh

mutate:
	python3 -I scripts/mutate.py

teeth:
	scripts/gates-have-teeth.sh
