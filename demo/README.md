# Demo recordings

`demo-en.tape` and `demo-ja.tape` record the GIFs in the top-level README with [vhs](https://github.com/charmbracelet/vhs).
They play a tour that was written beforehand, so recording does not call an LLM.

```sh
mise install                      # go, vhs, ttyd
./demo/record.sh                  # writes demo/demo-en.gif and demo/demo-ja.gif
```

`record.sh` clones [diff-tour-example](https://github.com/init-co-jp/diff-tour-example) into a temporary directory,
puts `tours/example-coupons.<lang>.json` into a temporary cache so that `tour` opens it instantly, and runs vhs.
